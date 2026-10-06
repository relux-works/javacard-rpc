package codegen

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Real jCardSim executes the adapter through Applet.process. Reset/deselect
// clear real transient arrays while persistent workspace retains bytes until
// generated cleanup. This does not prove physical NVM endurance or channel
// selection: the exhaustive CLA lane calls the production adapter directly.
func TestGeneratedWorkspaceRealSimulator(t *testing.T) {
	jar := simulatorJar(t)
	for _, memory := range []StreamMemory{StreamMemoryClearOnDeselect, StreamMemoryClearOnReset} {
		for _, policy := range []string{"transient", "persistent"} {
			t.Run(policy+"/"+string(memory), func(t *testing.T) {
				r, e := GenerateJavaSkeletonWithOptions(workspaceSchema(t, policy), "io.jcrpc.streamdemo.server", JavaOptions{StreamMemory: memory})
				if e != nil {
					t.Fatal(e)
				}
				fixture := strings.Replace(streamDemoAppletFixture, "private final Logic logic;", "static StreamDemoApplet installed;\n    private final Logic logic;", 1)
				fixture = strings.Replace(fixture, "StreamDemoApplet() {", "StreamDemoApplet() {\n        installed = this;", 1)
				harness := strings.ReplaceAll(workspaceLifecycleHarness, "PERSISTENT", fmt.Sprint(policy == "persistent"))
				harness = strings.ReplaceAll(harness, "e.sw", "e.getReason()")
				harness = strings.Replace(harness, "static StreamDemoApplet applet;", `static com.licel.jcardsim.base.Simulator simulator;
 static com.licel.jcardsim.base.SimulatorRuntime runtime;
 static javacard.framework.AID aid;
 static StreamDemoApplet applet;`, 1)
				harness = strings.Replace(harness, "applet=new StreamDemoApplet();", `runtime=new com.licel.jcardsim.base.SimulatorRuntime();
  simulator=new com.licel.jcardsim.base.Simulator(runtime);
  aid=new javacard.framework.AID(new byte[]{(byte)0xF0,0,0,1,2,1},(short)0,(byte)6);
  simulator.installApplet(aid,StreamDemoApplet.class);simulator.selectApplet(aid);applet=StreamDemoApplet.installed;`, 1)
				harness = strings.ReplaceAll(harness, "JCSystem.arrays.contains(workspace)", "(JCSystem.isTransient(workspace)!=JCSystem.NOT_A_TRANSIENT_OBJECT)")
				harness = strings.ReplaceAll(harness, "arrays=JCSystem.arrays.size();", "arrays=0;")
				harness = strings.ReplaceAll(harness, "!JCSystem.arrays.contains(field(runtime,n))", "JCSystem.isTransient(field(runtime,n))==JCSystem.NOT_A_TRANSIENT_OBJECT")
				// Actual deselection via selecting a second installed applet, then reselect.
				harness = strings.Replace(harness, "if(cleanup==1)applet.deselect();", `if(cleanup==1){
   javacard.framework.AID other=new javacard.framework.AID(new byte[]{(byte)0xF0,0,0,1,2,2},(short)0,(byte)6);
   simulator.installApplet(other,EmptyApplet.class);simulator.selectApplet(other);simulator.selectApplet(aid);
  }`, 1)
				harness = strings.ReplaceAll(harness, "JCSystem.reset();", "simulator.reset();simulator.selectApplet(aid);")
				harness = strings.Replace(harness, "if(JCSystem.arrays.size()!=arrays)throw new AssertionError(\"per-command transient allocation\");", "", 1)
				old := `static byte[] send(int ins,int p1,int p2,byte[] data){APDU a=new APDU((byte)0xB0,(byte)ins,(byte)p1,(byte)p2,data,data.length==0?new int[0]:new int[]{data.length});applet.process(a);return a.outgoing;}`
				new := `static byte[] send(int ins,int p1,int p2,byte[] data){
   byte[] command=new byte[data.length==0?4:5+data.length];command[0]=(byte)0xB0;command[1]=(byte)ins;command[2]=(byte)p1;command[3]=(byte)p2;
   if(data.length!=0){command[4]=(byte)data.length;System.arraycopy(data,0,command,5,data.length);}
   byte[] response=simulator.transmitCommand(command);int n=response.length;short sw=(short)(((response[n-2]&255)<<8)|(response[n-1]&255));
   if(sw!=(short)0x9000)ISOException.throwIt(sw);return Arrays.copyOf(response,n-2);
  }
  public static class EmptyApplet extends javacard.framework.Applet{
   public static void install(byte[] b,short o,byte l){new EmptyApplet().register();}
   public void process(APDU apdu){}
  }`
				if !strings.Contains(harness, old) {
					t.Fatal("send replacement not applied")
				}
				harness = strings.Replace(harness, old, new, 1)
				output := runRealJava(t, jar, r, fixture, harness, "WorkspaceHarness")
				t.Log(output)
			})
		}
	}
}

func simulatorJar(t *testing.T) string {
	t.Helper()
	jar := os.Getenv("JCRPC_JCARDSIM_JAR")
	if jar == "" {
		t.Skip("set JCRPC_JCARDSIM_JAR for real simulator tests")
	}
	if _, e := os.Stat(jar); e != nil {
		t.Fatal(e)
	}
	return jar
}
func runRealJava(t *testing.T, jar string, r *JavaGenerationResult, fixture, harness, main string) string {
	t.Helper()
	output, err := runRealJavaResult(t, jar, r, fixture, harness, main)
	if err != nil {
		t.Fatalf("real Java run: %v\n%s", err, output)
	}
	return output
}

func runRealJavaResult(t *testing.T, jar string, r *JavaGenerationResult, fixture, harness, main string) (string, error) {
	t.Helper()
	root := t.TempDir()
	pkg := regexp.MustCompile(`package ([^;]+);`).FindSubmatch(r.SkeletonSource)
	packageName := string(pkg[1])
	dir := filepath.Join(root, strings.ReplaceAll(packageName, ".", "/"))
	if e := os.MkdirAll(dir, 0755); e != nil {
		t.Fatal(e)
	}
	fixtureName := regexp.MustCompile(`public final class (\w+)`).FindStringSubmatch(fixture)[1]
	files := map[string][]byte{r.TransportName: r.TransportSource, r.SkeletonName: r.SkeletonSource, r.StreamEndpointName: r.StreamEndpointSource, r.StreamRuntimeName: r.StreamRuntimeSource, r.StreamAPDUAdapterName: r.StreamAPDUAdapterSource, fixtureName: []byte(fixture), main: []byte(harness)}
	args := []string{"-cp", jar, "-d", root}
	for name, source := range files {
		path := filepath.Join(dir, name+".java")
		if e := os.WriteFile(path, source, 0644); e != nil {
			t.Fatal(e)
		}
		args = append(args, path)
	}
	output, e := exec.Command("javac", args...).CombinedOutput()
	if e != nil {
		t.Fatalf("real Java compile: %v\n%s", e, output)
	}
	output, e = exec.Command("java", "-cp", root+string(os.PathListSeparator)+jar, packageName+"."+main).CombinedOutput()
	return string(output), e
}

// Measure actual arrays reachable from the generated bsim-auth skeleton and
// adapter after real simulator installation, including static dispatch tables
// and all status arrays. Byte/short payload is exact; reference slots have no
// claimed physical width. Provider/JCRE objects and object headers are excluded.
func TestBSimAuthGeneratedAllocationPayload(t *testing.T) {
	jar := simulatorJar(t)
	idl := os.Getenv("JCRPC_ALLOCATION_IDL")
	if idl == "" {
		t.Skip("set JCRPC_ALLOCATION_IDL to the read-only bsim-auth IDL")
	}
	s, e := ParseFile(idl)
	if e != nil {
		t.Fatal(e)
	}
	for _, policy := range []string{"baseline", "transient", "persistent"} {
		t.Run(policy, func(t *testing.T) {
			var r *JavaGenerationResult
			if policy == "baseline" {
				dir := os.Getenv("JCRPC_ALLOCATION_BASELINE")
				if dir == "" {
					t.Fatal("JCRPC_ALLOCATION_BASELINE required")
				}
				r = &JavaGenerationResult{TransportName: "BSimAuthTransport", SkeletonName: "BSimAuthSkeleton", StreamEndpointName: "BSimAuthStreamEndpoint", StreamRuntimeName: "BSimAuthBoundedStreamRuntime", StreamAPDUAdapterName: "BSimAuthStreamAPDUAdapter"}
				for name, target := range map[string]*[]byte{r.TransportName: &r.TransportSource, r.SkeletonName: &r.SkeletonSource, r.StreamEndpointName: &r.StreamEndpointSource, r.StreamRuntimeName: &r.StreamRuntimeSource, r.StreamAPDUAdapterName: &r.StreamAPDUAdapterSource} {
					b, e := os.ReadFile(filepath.Join(dir, name+".java"))
					if e != nil {
						t.Fatal(e)
					}
					*target = b
				}
			} else {
				s.Applet.StreamWorkspace = policy
				r, e = GenerateJavaSkeleton(s, "io.jcrpc.bsim")
				if e != nil {
					t.Fatal(e)
				}
			}
			signatures := regexp.MustCompile(`protected abstract ([\s\S]*?);`).FindAllStringSubmatch(string(r.SkeletonSource), -1)
			var methods strings.Builder
			for _, signature := range signatures {
				body := "return (short)0;"
				if strings.HasPrefix(signature[1], "byte[]") {
					body = "return null;"
				}
				fmt.Fprintf(&methods, "protected %s { %s }\n", signature[1], body)
			}
			fixture := strings.ReplaceAll(allocationApplet, "METHODS", methods.String())
			output := runRealJava(t, jar, r, fixture, allocationHarness, "AllocationHarness")
			workspace := 255
			for _, method := range s.Methods {
				for _, message := range []*Message{method.Request, method.Response} {
					if message != nil {
						for _, field := range message.Fields {
							if field.Type == FieldTypeStream && field.MaxLength > workspace {
								workspace = field.MaxLength
							}
						}
					}
				}
			}
			// Independent inventory: scratch 32 + scalars 17*2 + three status
			// words 3*2 + adapter 255 = 327; tables 9+9+45*2 = 108.
			transient, persistent, arrays := workspace+327, 108, 12
			if policy == "persistent" {
				transient, persistent, arrays = 328, 108+workspace, 13
			}
			want := fmt.Sprintf("TOTAL transient primitive payload=%d B; transient reference slots=1; persistent primitive payload=%d B; persistent reference slots=0; arrays=%d", transient, persistent, arrays)
			if !strings.Contains(output, want) {
				t.Fatalf("allocation inventory mismatch, want %s\n%s", want, output)
			}
			t.Logf("%s\n%s", policy, output)
		})
	}
}

const allocationApplet = `package io.jcrpc.bsim;
import javacard.framework.*;
public final class AllocationApplet extends Applet {
 static AllocationApplet installed;
 final Logic logic=new Logic();final BSimAuthStreamAPDUAdapter adapter=new BSimAuthStreamAPDUAdapter(logic);
 public static void install(byte[] b,short o,byte l){new AllocationApplet().register();}
 AllocationApplet(){installed=this;}
 public void process(APDU apdu){if(selectingApplet())return;adapter.processIfStream(apdu);}
 public void deselect(){adapter.deselect();}
 static final class Logic extends BSimAuthSkeleton {
  Logic(){super(null);}
  METHODS
 }
}
`
const allocationHarness = `package io.jcrpc.bsim;
import java.lang.reflect.*;
import java.util.*;
import javacard.framework.*;
import com.licel.jcardsim.base.*;
public class AllocationHarness {
 static final Set<Object> seen=Collections.newSetFromMap(new IdentityHashMap<Object,Boolean>());
 static int transientBytes,persistentBytes,transientSlots,persistentSlots,arrayCount;
 public static void main(String[] args)throws Exception{
  Simulator simulator=new Simulator();AID aid=new AID(new byte[]{(byte)0xF0,0,0,0,0x20,1},(short)0,(byte)6);
  simulator.installApplet(aid,AllocationApplet.class);simulator.selectApplet(aid);
  walk(AllocationApplet.installed.logic,"skeleton");walk(AllocationApplet.installed.adapter,"adapter");
  System.out.println("TOTAL transient primitive payload="+transientBytes+" B; transient reference slots="+transientSlots+"; persistent primitive payload="+persistentBytes+" B; persistent reference slots="+persistentSlots+"; arrays="+arrayCount);
  // Positive control: an extra allocated transient array is classified and counted.
  int before=transientBytes;walk(JCSystem.makeTransientByteArray((short)7,JCSystem.CLEAR_ON_RESET),"control");
  if(transientBytes-before!=7)throw new AssertionError("allocation instrument control");
 }
 static void walk(Object x,String path)throws Exception{
  if(x==null||!seen.add(x))return;
  Class<?> c=x.getClass();if(c.isArray()){
   int n=Array.getLength(x);boolean transientArray=JCSystem.isTransient(x)!=JCSystem.NOT_A_TRANSIENT_OBJECT;
   Class<?> component=c.getComponentType();int bytes=component==byte.class?n:component==short.class?2*n:0;
   int slots=component.isPrimitive()?0:n;
   if(component.isPrimitive() && component!=byte.class && component!=short.class)throw new AssertionError("unexpected array component");
   if(transientArray){transientBytes+=bytes;transientSlots+=slots;}else{persistentBytes+=bytes;persistentSlots+=slots;}arrayCount++;
   System.out.println(path+": "+(transientArray?"transient":"persistent")+" "+component.getName()+"["+n+"] payload="+bytes+" B slots="+slots);
   return;
  }
  while(c!=null && c.getName().startsWith("io.jcrpc.bsim.")){
   for(Field f:c.getDeclaredFields()){if(f.getType().isPrimitive())continue;f.setAccessible(true);walk(f.get(Modifier.isStatic(f.getModifiers())?null:x),path+"."+f.getName());}c=c.getSuperclass();
  }
 }
}
`

// A bounded instrumentation plant misclassifies only transient byte[7] as NVM.
// The positive seven-byte control must catch it by its named assertion; an
// unrelated compile/install failure is not evidence that accounting works.
func TestAllocationInstrumentRejectsNarrowingPlant(t *testing.T) {
	jar := simulatorJar(t)
	s := workspaceSchema(t, "persistent")
	s.Applet.Name = "BSimAuth"
	r, e := GenerateJavaSkeleton(s, "io.jcrpc.bsim")
	if e != nil {
		t.Fatal(e)
	}
	signatures := regexp.MustCompile(`protected abstract ([\s\S]*?);`).FindAllStringSubmatch(string(r.SkeletonSource), -1)
	var methods strings.Builder
	for _, signature := range signatures {
		body := "return (short)0;"
		if strings.HasPrefix(signature[1], "byte on") {
			body = "return (byte)0;"
		}
		fmt.Fprintf(&methods, "protected %s { %s }\n", signature[1], body)
	}
	fixture := strings.ReplaceAll(allocationApplet, "METHODS", methods.String())
	if output, e := runRealJavaResult(t, jar, r, fixture, allocationHarness, "AllocationHarness"); e != nil {
		t.Fatalf("positive control: %v\n%s", e, output)
	}
	mutant := strings.Replace(allocationHarness, "boolean transientArray=JCSystem.isTransient(x)!=JCSystem.NOT_A_TRANSIENT_OBJECT;", "boolean transientArray=JCSystem.isTransient(x)!=JCSystem.NOT_A_TRANSIENT_OBJECT && !(x instanceof byte[] && n==7);", 1)
	if mutant == allocationHarness {
		t.Fatal("plant not applied")
	}
	output, e := runRealJavaResult(t, jar, r, fixture, mutant, "AllocationHarness")
	if e == nil || !strings.Contains(output, "AssertionError: allocation instrument control") {
		t.Fatalf("plant escaped named assertion: %v\n%s", e, output)
	}
	t.Log("killed transient-byte-seven-misclassification: allocation instrument control")
}
