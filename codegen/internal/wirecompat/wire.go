// Package wirecompat exercises the exact Java corpus emitted by the CLI.
// It is shared by acceptance tests, not linked into the generator.
package wirecompat

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/relux-works/javacard-rpc/pluginapi"
)

// Compare preserves the breaking Java API while requiring both independently
// emitted corpora to satisfy IDL-grounded wire probes and emit identical traces.
// JCRE stand-ins are bounded to wire framing; this is no physical-card claim.
func Compare(t testing.TB, old, now string, schema *pluginapi.Schema) error {
	t.Helper()
	a, err := Observe(t, old, schema)
	if err != nil {
		return fmt.Errorf("baseline: %w", err)
	}
	b, err := Observe(t, now, schema)
	if err != nil {
		return fmt.Errorf("candidate: %w", err)
	}
	if !bytes.Equal(a, b) {
		return fmt.Errorf("Java behavioral trace drift:\nold %s\nnew %s", a, b)
	}
	return nil
}

var abstract = regexp.MustCompile(`protected abstract (\w+) (\w+)\(([^;]*)\);`)
var packageLine = regexp.MustCompile(`(?m)^package [^;]+;`)

// Observe compiles copies of every emitted Java file, never a regenerated
// substitute. Only package declarations are relocated, so accepted POSIX
// namespaces that javac cannot spell can exercise the same method bodies.
// Expected routing/widths come from the input IDL, never generated INS tokens.
func Observe(t testing.TB, corpus string, schema *pluginapi.Schema) ([]byte, error) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{}
	var skeleton string
	err := filepath.WalkDir(corpus, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".java") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		name := filepath.Base(path)
		if _, ok := files[name]; ok {
			return fmt.Errorf("duplicate Java file %s", name)
		}
		source := packageLine.ReplaceAllString(string(raw), "package wireprobe;")
		files[name] = source
		if name == schema.Applet.Name+"Skeleton.java" {
			skeleton = source
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if skeleton == "" {
		return nil, fmt.Errorf("missing emitted skeleton")
	}
	harness, err := probe(schema, skeleton)
	if err != nil {
		return nil, err
	}
	files["WireProbe.java"] = harness
	for name, source := range framework {
		files[name] = source
	}
	paths := []string{}
	for name, source := range files {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(source), 0644); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	if b, err := command(t, "javac", append([]string{"-d", root}, paths...)...); err != nil {
		return nil, fmt.Errorf("compile is not a wire kill: %w\n%s", err, b)
	}
	b, err := command(t, "java", "-cp", root, "wireprobe.WireProbe")
	if err != nil {
		return nil, fmt.Errorf("behavior: %w\n%s", err, b)
	}
	return b, nil
}

func command(t testing.TB, name string, args ...string) ([]byte, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	b, err := cmd.CombinedOutput()
	exit := -1
	if cmd.ProcessState != nil {
		exit = cmd.ProcessState.ExitCode()
	}
	t.Logf("wire command %s %q exit=%d\n%s", name, args, exit, b)
	return b, err
}

func width(message *pluginapi.Message) (int, bool) {
	if message == nil {
		return 0, true
	}
	n := 0
	for _, f := range message.Fields {
		w, fixed := f.WireSize()
		if !fixed {
			return 3, false
		}
		n += w
	}
	return n, true
}

func probe(schema *pluginapi.Schema, skeleton string) (string, error) {
	name := schema.Applet.Name
	methods := map[string]*pluginapi.Method{}
	keys := []string{}
	stream := false
	for key, m := range schema.Methods {
		keys = append(keys, key)
		stream = stream || m.HasStream()
	}
	sort.Strings(keys)
	for _, key := range keys {
		m := schema.Methods[key]
		callback := "on" + strings.ToUpper(key[:1]) + key[1:]
		if m.HasStream() {
			callback += "Stream"
		}
		methods[callback] = m
	}
	var impl, checks strings.Builder
	for _, a := range abstract.FindAllStringSubmatch(skeleton, -1) {
		m := methods[a[2]]
		if m == nil {
			return "", fmt.Errorf("unmapped callback %s", a[2])
		}
		fmt.Fprintf(&impl, "protected %s %s(%s) { calls++; last=%d; if(fail) throw statusWordFailure((short)0x6986);", a[1], a[2], a[3], m.INS)
		if strings.Contains(a[3], "byte[] output") {
			n, _ := width(m.Response)
			if m.Response.StreamField() != nil {
				n = 3
			}
			fmt.Fprintf(&impl, "short n=(short)%d; if(n<=outputCapacity) for(short i=0;i<n;i++) output[(short)(outputOffset+i)]=(byte)(%d+i); return n;", n, m.INS)
		} else {
			switch a[1] {
			case "void":
			case "boolean":
				impl.WriteString("return true;")
			case "byte", "short", "int":
				fmt.Fprintf(&impl, "return (%s)7;", a[1])
			default:
				return "", fmt.Errorf("unsupported return %s", a[1])
			}
		}
		impl.WriteString("}\n")
	}
	fmt.Fprintf(&checks, "check(cla()==%d,\"CLA constant\");\n", schema.Applet.CLA)
	for _, key := range keys {
		m := schema.Methods[key]
		if m.HasStream() {
			n, _ := width(m.Response)
			if m.Response.StreamField() != nil {
				n = 3
			}
			req := m.Request.StreamField() != nil
			resp := m.Response.StreamField() != nil
			fmt.Fprintf(&checks, "stream(%d,%t,%t,%d);\n", m.INS, req, resp, n)
			continue
		}
		request := 0
		variable := false
		if m.Request != nil {
			for _, f := range m.Request.Fields {
				if f.Location == pluginapi.ParameterLocationP1 || f.Location == pluginapi.ParameterLocationP2 {
					continue
				}
				w, fixed := f.WireSize()
				if !fixed {
					w = 3
					variable = true
				}
				request += w
			}
		}
		n, fixed := width(m.Response)
		// Callback results are a constant scalar 7 or an INS-tagged buffer. The
		// callback marker also detects routing swaps between equal-width methods.
		scalar := 0
		if m.Response != nil && len(m.Response.Fields) == 1 {
			switch m.Response.Fields[0].Type {
			case pluginapi.FieldTypeU8, pluginapi.FieldTypeU16, pluginapi.FieldTypeU32:
				scalar = 7
			case pluginapi.FieldTypeBool:
				scalar = 1
			}
		}
		fmt.Fprintf(&checks, "ordinary(%d,%d,%d,%d);\n", m.INS, request, n, scalar)
		// All supported fixed ordinary request spans reject an extra byte before
		// callback effects; variable input has no invented generic business minimum.
		if !variable {
			fmt.Fprintf(&checks, "badRequest(%d,%d);\n", m.INS, request+1)
		}
		if fixed && n > 0 {
			fmt.Fprintf(&checks, "shortOutput(%d,%d,%d);\n", m.INS, request, n-1)
		}
	}
	if stream {
		var families strings.Builder
		for _, key := range keys {
			m := schema.Methods[key]
			if m.HasStream() {
				fmt.Fprintf(&families, "(i>=%d&&i<%d)||", m.INS, int(m.INS)+6)
			}
		}
		fmt.Fprintf(&checks, "for(int i=0;i<256;i++) check(l.isStreamInstruction((byte)i)==(%sfalse),\"stream INS family \"+i);\n", families.String())
	}
	var supported strings.Builder
	for _, key := range keys {
		m := schema.Methods[key]
		if !m.HasStream() {
			fmt.Fprintf(&supported, "i==%d||", m.INS)
		}
	}
	checks.WriteString("for(int i=0;i<256;i++) if(!(" + supported.String() + "false)) unknown(i);\n")
	for key, sw := range schema.StatusWords {
		fmt.Fprintf(&checks, "check((%sSkeleton.%s & 65535)==%d,\"IDL status %s\");\n", name, key, sw.Code, key)
	}
	source := strings.NewReplacer("@NAME@", name, "@IMPL@", impl.String(), "@CHECKS@", checks.String(), "@CLA@", fmt.Sprint(schema.Applet.CLA)).Replace(javaProbe)
	scratch := ""
	if strings.Contains(skeleton, "byte[] callerWorkspace") {
		scratch = ",new byte[0],(short)0,(short)0"
	}

	if stream {
		source = strings.ReplaceAll(source, "@STREAM@", strings.ReplaceAll(javaStream, "@NAME@", name))
	} else {
		source = strings.ReplaceAll(source, "@STREAM@", "")
	}
	source = strings.NewReplacer("@SCRATCH@", scratch, "@CLA@", fmt.Sprint(schema.Applet.CLA)).Replace(source)
	return source, nil
}

const javaProbe = `package wireprobe;
import java.util.Arrays;
public class WireProbe {
 static class Logic extends @NAME@Skeleton {
  int calls,last; boolean fail;
  Logic(){super((i,p1,p2,d)->new byte[0]);}
  @IMPL@
 }
 static Logic l=new Logic();
 static void check(boolean ok,String label){if(!ok)throw new AssertionError("wire contract: "+label);}
 static int cla() throws Exception { for(java.lang.reflect.Field f:@NAME@Skeleton.class.getFields()) if(f.getName().startsWith("CLA_"))return f.getByte(null)&255; throw new AssertionError("missing CLA"); }
 static short call(int ins,byte[] in,byte[] out,int cap){return l.dispatchTo((byte)ins,(byte)0,(byte)0,in,(short)0,(short)in.length,out,(short)0,(short)cap@SCRATCH@);}
 static void ordinary(int ins,int request,int width,int scalar){
  byte[] in=new byte[request],out=new byte[255];int before=l.calls;String label=String.format("ordinary INS%02X",ins);
  short n;
  try {n=call(ins,in,out,255);}catch(@NAME@Skeleton.StatusWordException e){throw new AssertionError("wire contract: "+label+" status "+Integer.toHexString(e.getStatusWord()&65535)+" want 9000");}
  check(n==width,label+" response length");check(l.calls==before+1 && l.last==ins,label+" callback");
  if(scalar>0 && width>0){for(int i=0;i<width;i++)check(out[i]==(i==width-1?scalar:0),label+" scalar bytes");}
  else for(int i=0;i<width;i++)check(out[i]==(byte)(ins+i),label+" output bytes");
  l.fail=true;expect(ins,in,255,0x6986,false,label+" business SW");l.fail=false;
  System.out.println(label+":9000:"+n+":"+Arrays.toString(Arrays.copyOf(out,n)));
 }
 static void expect(int ins,byte[] in,int cap,int sw,boolean noCall,String label){
  int before=l.calls;byte[] out=new byte[255];Arrays.fill(out,(byte)0x55);
  try {call(ins,in,out,cap);throw new AssertionError("wire contract: "+label+" admitted");}catch(@NAME@Skeleton.StatusWordException e){check((e.getStatusWord()&65535)==sw,label+" status");}
  if(noCall)check(l.calls==before,label+" callback effect");
  for(byte b:out)check(b==(byte)0x55,label+" output effect");
 }
 static void badRequest(int ins,int request){expect(ins,new byte[request],255,0x6700,true,"wrong request length");}
 static void shortOutput(int ins,int request,int cap){expect(ins,new byte[request],cap,0x6700,true,"short response capacity");}
 static void unknown(int ins){expect(ins,new byte[0],255,0x6D00,true,"unknown ordinary INS"+ins);}
 @STREAM@
 public static void main(String[] args)throws Exception { @CHECKS@ System.out.println("WIRE_OK"); }
}
`

const javaStream = `
 static @NAME@StreamAPDUAdapter adapter;
 static byte[] frame(int cla,int ins,int p1,int p2,byte[] data,int sw,int length,String label){
  javacard.framework.APDU a=new javacard.framework.APDU((byte)cla,(byte)ins,(byte)p1,(byte)p2,data);
  int before=l.calls;
  try {check(adapter.processIfStream(a),label+" stream not recognized");check(sw==0x9000,label+" admitted");}
  catch(javacard.framework.ISOException e){check((e.sw&65535)==sw,label+" status "+Integer.toHexString(e.sw&65535));check(sw!=0x9000,label+" failed valid frame");check(l.calls==before,label+" refusal called handler");}
  check(a.outgoing.length==length,label+" response length");return a.outgoing;
 }
 static byte[] close(byte[] data)throws Exception {
  byte[] result=new byte[34];result[0]=(byte)(data.length>>8);result[1]=(byte)data.length;
  System.arraycopy(java.security.MessageDigest.getInstance("SHA-256").digest(data),0,result,2,32);return result;
 }
 static void stream(int ins,boolean request,boolean response,int width)throws Exception {
  l=new Logic();adapter=new @NAME@StreamAPDUAdapter(l);byte[] empty=new byte[0],input=new byte[]{1,2,3};
  frame(@CLA@^0x10,ins,0,request?1:0,request?input:empty,0x6E00,0,"stream CLA refusal");
  // Both request-bearing CLOSE_WRITE and response-only invoke execute the
  // actual endpoint/runtime and generated Handler, once per successful flow.
  byte[] descriptor;
  if(request){frame(@CLA@,ins,0,1,input,0x9000,0,"stream write");descriptor=frame(@CLA@,ins+1,0,0,close(input),0x9000,response?35:width,"stream closeWrite");}
  else descriptor=frame(@CLA@,ins,0,0,empty,0x9000,response?35:width,"stream invoke");
  check(l.calls==1&&l.last==ins,"stream callback INS"+ins);
  if(response){
   check((descriptor[0]&255)==1 && (descriptor[1]&255)==0 && (descriptor[2]&255)==width,"stream descriptor width");
   byte[] pending=frame(@CLA@,ins+2,0,0,empty,0x9000,35,"stream pending");check(Arrays.equals(pending,descriptor),"pending descriptor bytes");
   byte[] out=frame(@CLA@,ins+3,0,1,empty,0x9000,width,"stream read");for(int i=0;i<width;i++)check(out[i]==(byte)(ins+i),"stream result bytes");
   frame(@CLA@,ins+4,0,0,close(out),0x9000,0,"stream closeRead");check(l.calls==1,"read executes again");
  }
  frame(@CLA@,ins+5,0,0,empty,0x9000,0,"stream abort");
  frame(@CLA@,ins+2,0,0,empty,0x6985,0,"no pending result");
  // Unknown stream dispatch must refuse; ordinary paths remain unclaimed.
  javacard.framework.APDU unknown=new javacard.framework.APDU((byte)@CLA@,(byte)0xFF,(byte)0,(byte)0,empty);
  check(!adapter.processIfStream(unknown)&&unknown.outgoing.length==0,"unknown stream adapter effects");
  try{l.dispatchStreamTo((byte)0xFF,(byte)0,(byte)0,empty,(short)0,(short)0,new byte[255],(short)0,(short)255@SCRATCH@);throw new AssertionError("wire contract: unknown stream admitted");}
  catch(javacard.framework.ISOException e){check((e.sw&65535)==0x6D00,"unknown stream SW");}
  claSweep(ins);
  System.out.println("stream INS"+ins+":9000:"+width+":CLA6e00:pending6985");
 }
 static void claSweep(int ins){
  l=new Logic();adapter=new @NAME@StreamAPDUAdapter(l);byte[] empty=new byte[0];
  StringBuilder classes=new StringBuilder();
  for(int c=0;c<256;c++){
   javacard.framework.APDU a=new javacard.framework.APDU((byte)c,(byte)(ins+5),(byte)0,(byte)0,empty);
   int sw=0x9000;
   try{check(adapter.processIfStream(a),"CLA sweep unclaimed stream");}
   catch(javacard.framework.ISOException e){sw=e.sw&65535;}
   check(sw==0x9000||sw==0x6E00,"CLA sweep unrelated status");
   check(a.outgoing.length==0 && l.calls==0,"CLA sweep effects");
   classes.append(Integer.toHexString(sw)).append(',');
  }
  System.out.println("stream CLA sweep "+ins+":"+classes);
 }
`

var framework = map[string]string{
	"JCSystem.java":      `package javacard.framework; public final class JCSystem { public static final byte CLEAR_ON_RESET=0,CLEAR_ON_DESELECT=1; public static byte[] makeTransientByteArray(short n,byte e){return new byte[n];} public static short[] makeTransientShortArray(short n,byte e){return new short[n];} public static Object[] makeTransientObjectArray(short n,byte e){return new Object[n];} }`,
	"ISO7816.java":       `package javacard.framework; public interface ISO7816 { short OFFSET_CLA=0,OFFSET_INS=1,OFFSET_P1=2,OFFSET_P2=3,OFFSET_CDATA=5; short SW_CLA_NOT_SUPPORTED=(short)0x6E00,SW_WRONG_LENGTH=(short)0x6700,SW_INS_NOT_SUPPORTED=(short)0x6D00; }`,
	"ISOException.java":  `package javacard.framework; public class ISOException extends RuntimeException { public short sw; ISOException(short sw){this.sw=sw;} public static void throwIt(short sw){throw new ISOException(sw);} }`,
	"APDU.java":          `package javacard.framework; public class APDU { byte[] buffer=new byte[260],data; int offset; public byte[] outgoing=new byte[0]; public APDU(byte cla,byte ins,byte p1,byte p2,byte[] data){buffer[0]=cla;buffer[1]=ins;buffer[2]=p1;buffer[3]=p2;this.data=data;} public byte[] getBuffer(){return buffer;} public short getIncomingLength(){return (short)data.length;} public short setIncomingAndReceive(){return receiveBytes((short)5);} public short receiveBytes(short at){int n=Math.min(2,data.length-offset);System.arraycopy(data,offset,buffer,at,n);offset+=n;return (short)n;} public short setOutgoing(){return 0;} public void setOutgoingLength(short n){} public void sendBytesLong(byte[] b,short at,short n){outgoing=java.util.Arrays.copyOfRange(b,at,at+n);} }`,
	"MessageDigest.java": `package javacard.security; public class MessageDigest { public static final byte ALG_SHA_256=4; public static MessageDigest getInstance(byte a,boolean b){return new MessageDigest();} public short doFinal(byte[] in,short at,short n,byte[] out,short to){try{java.security.MessageDigest d=java.security.MessageDigest.getInstance("SHA-256");d.update(in,at,n);byte[] hash=d.digest();System.arraycopy(hash,0,out,to,32);return 32;}catch(Exception e){throw new RuntimeException(e);}} }`,
}
