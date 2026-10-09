package codegen

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func facadeMixedFixture(t *testing.T, width int, policy string) (*JavaGenerationResult, string) {
	t.Helper()
	s := workspaceSchema(t, policy)
	s.Methods["identity"] = &Method{Name: "identity", INS: 2, Response: &Message{Fields: []Field{{Name: "identity", Type: FieldTypeBytesFixed, FixedLength: width}}}}
	r, e := GenerateJavaSkeleton(s, "io.jcrpc.streamdemo.server")
	if e != nil {
		t.Fatal(e)
	}
	fixture := strings.Replace(streamDemoAppletFixture, "private static final class Logic", "static final class Logic", 1)
	fixture = strings.Replace(fixture, "private final Logic logic;", "static StreamDemoApplet installed;\n    final Logic logic;", 1)
	fixture = strings.Replace(fixture, "StreamDemoApplet() {", "StreamDemoApplet() { installed=this;", 1)
	fixture = strings.Replace(fixture, "Logic() { super(new NoopTransport()); }", `Logic() { super(new NoopTransport()); }
        protected short onIdentity(byte[] output,short off,short cap,byte[] scratch,short scratchOff,short scratchCap){
            if(cap!=WIDTH)throw statusWordFailure((short)0x6700);
            if(scratchCap>0)scratch[scratchOff]=(byte)scratchCap;
            for(short i=0;i<cap;i++)output[(short)(off+i)]=(byte)i;
            return cap;
        }`, 1)
	// Counter owns its full receive logic; this fixed/no-data mixed entry needs
	// only captured headers and zero incoming bytes. It never borrows global APDU.
	fixture = strings.Replace(fixture, "ISOException.throwIt(ISO7816.SW_INS_NOT_SUPPORTED);", `byte[] buffer=apdu.getBuffer();
        byte ins=buffer[1],p1=buffer[2],p2=buffer[3];
        short length=apdu.setIncomingAndReceive();
        if(length!=0)ISOException.throwIt((short)0x6700);
        try {
            short produced=logic.dispatchTo(ins,p1,p2,buffer,(short)5,(short)0,
                    buffer,(short)0,(short)buffer.length,buffer,(short)0,(short)buffer.length);
            apdu.setOutgoing();apdu.setOutgoingLength(produced);apdu.sendBytesLong(buffer,(short)0,produced);
        } catch(StreamDemoSkeleton.StatusWordException e){ISOException.throwIt(e.getStatusWord());}`, 1)
	// Observe the independent scratch through both execution paths. The callback
	// itself stores only primitive counters; the incoming command's array is local.
	fixture = strings.ReplaceAll(fixture, "short callerWorkspaceCapacity) {", `short callerWorkspaceCapacity) {
            if(callerWorkspaceCapacity>0)callerWorkspace[callerWorkspaceOffset]=(byte)callerWorkspaceCapacity;`)
	fixture = strings.ReplaceAll(fixture, "WIDTH", fmt.Sprint(width))
	return r, fixture
}

// Real JVM classes execute mixed ordinary/stream dispatch with disjoint spans,
// nonzero/end/overflow windows, exact 127/190/177 replies and stream retry after
// invalid scratch. This is framework/fixture behavior, not Auth phase policy.
func TestFacadeMixedCallerWorkspaceDispatch(t *testing.T) {
	jar := simulatorJar(t)
	for _, width := range []int{127, 190, 177} {
		for _, policy := range []string{"transient", "persistent"} {
			t.Run(fmt.Sprintf("%d/%s", width, policy), func(t *testing.T) {
				r, fixture := facadeMixedFixture(t, width, policy)
				harness := strings.ReplaceAll(facadeMixedHarness, "WIDTH", fmt.Sprint(width))
				runRealJava(t, jar, r, fixture, harness, "MixedHarness")
			})
		}
	}
}

// The installed mixed applet uses Simulator.transmitCommand -> process for
// all three fixed widths, CLOSE_WRITE and response-only invocation, then reads
// session-owned results. Physical APDU authority remains outside this claim.
func TestFacadeMixedCallerWorkspaceRealAPDU(t *testing.T) {
	jar := simulatorJar(t)
	for _, width := range []int{127, 190, 177} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			r, fixture := facadeMixedFixture(t, width, "")
			runRealJava(t, jar, r, fixture, strings.ReplaceAll(facadeMixedAPDUHarness, "WIDTH", fmt.Sprint(width)), "MixedAPDUHarness")
		})
	}
}

// Mixed ordinary and stream signatures convert and verify on the actual
// Classic kit with optional ints. Six widths/storage combinations are real CAPs.
func TestFacadeMixedCallerWorkspaceClassicCAP(t *testing.T) {
	ant, e := exec.LookPath("ant")
	if e != nil {
		t.Skip("ant unavailable")
	}
	task, kit := os.Getenv("JCRPC_ANT_JAVACARD_JAR"), os.Getenv("JCRPC_JCKIT_DIR")
	if task == "" || kit == "" {
		t.Skip("set real Classic CAP inputs")
	}
	for _, width := range []int{127, 190, 177} {
		for _, policy := range []string{"transient", "persistent"} {
			t.Run(fmt.Sprintf("%d/%s", width, policy), func(t *testing.T) {
				r, fixture := facadeMixedFixture(t, width, policy)
				convertFacadeFixtureCAP(t, ant, task, kit, r, fixture)
			})
		}
	}
}

const facadeMixedHarness = `package io.jcrpc.streamdemo.server;
import java.util.*;
public class MixedHarness {
 static void check(boolean ok,String message){if(!ok)throw new AssertionError(message);}
 static byte[] close(byte[] data)throws Exception{byte[] b=new byte[34];b[0]=(byte)(data.length>>8);b[1]=(byte)data.length;System.arraycopy(java.security.MessageDigest.getInstance("SHA-256").digest(data),0,b,2,32);return b;}
 static short stream(StreamDemoApplet.Logic logic,int ins,byte[] request,byte[] out,byte[] scratch,short off,short cap){return logic.dispatchStreamTo((byte)ins,(byte)0,(byte)(ins==0x20||ins==0x23||ins==0x33?1:0),request,(short)0,(short)request.length,out,(short)7,(short)35,scratch,off,cap);}
 public static void main(String[] args)throws Exception{
  com.licel.jcardsim.base.Simulator sim=new com.licel.jcardsim.base.Simulator();javacard.framework.AID aid=new javacard.framework.AID(new byte[]{(byte)0xF0,0,0,1,2,1},(short)0,(byte)6);sim.installApplet(aid,StreamDemoApplet.class);sim.selectApplet(aid);
  StreamDemoApplet.Logic logic=StreamDemoApplet.installed.logic;byte[] scratch=new byte[300],out=new byte[256];Arrays.fill(scratch,(byte)85);Arrays.fill(out,(byte)86);
  check(logic.dispatchTo((byte)2,(byte)0,(byte)0,null,(short)0,(short)0,out,(short)6,(short)WIDTH,scratch,(short)11,(short)260)==WIDTH,"mixed produced width");
  check(scratch[10]==85&&scratch[11]==(byte)260&&scratch[12]==85,"mixed scratch identity/span");
  check(out[5]==86&&out[6+WIDTH]==86,"mixed output neighbors");for(int i=0;i<WIDTH;i++)check(out[6+i]==(byte)i,"mixed wire");
  byte[] input={1,2,3};stream(logic,0x20,input,out,scratch,(short)9,(short)196);
  byte[] receipt=close(input),before=out.clone();
  // Invalid scratch must preserve the pending write, scratch and descriptor.
  for(int kind=0;kind<6;kind++){
   byte[] w=kind==0?null:scratch;short off=kind==1?(short)-1:kind==3?(short)300:kind==4?(short)32767:kind==5?(short)301:(short)9;
   short cap=kind==2?(short)-1:kind==4?(short)32767:kind==3?(short)1:(short)196;
   byte[] saved=scratch.clone();
   try{stream(logic,0x21,receipt,out,w,off,cap);throw new AssertionError("mixed invalid scratch admitted");}
   catch(javacard.framework.ISOException e){check((e.getReason()&65535)==0x6700,"mixed refusal status");}
   check(Arrays.equals(out,before)&&Arrays.equals(scratch,saved),"mixed invalid scratch partial effects");
  }
  check(stream(logic,0x21,receipt,out,scratch,(short)9,(short)196)==35,"mixed close retry");
  check(scratch[8]==85&&scratch[9]==(byte)196&&scratch[10]==85,"mixed close scratch span");
  check(stream(logic,0x22,new byte[0],out,scratch,(short)300,(short)0)==35,"mixed empty end control");
  check(stream(logic,0x23,new byte[0],out,scratch,(short)9,(short)196)==3,"mixed stream read");
  check(Arrays.equals(Arrays.copyOfRange(out,7,10),new byte[]{3,2,1}),"mixed result lifetime");
  stream(logic,0x24,close(new byte[]{3,2,1}),out,scratch,(short)9,(short)196);
  check(stream(logic,0x30,new byte[0],out,scratch,(short)17,(short)260)==35,"mixed response-only");
  check(scratch[16]==85&&scratch[17]==(byte)260&&scratch[18]==85,"mixed response scratch span");
  check(stream(logic,0x33,new byte[0],out,scratch,(short)17,(short)260)==4,"mixed response read");
  check(Arrays.equals(Arrays.copyOfRange(out,7,11),new byte[]{1,2,3,1}),"mixed response wire");
 }
}
`
const facadeMixedAPDUHarness = `package io.jcrpc.streamdemo.server;
import java.util.*;
import com.licel.jcardsim.base.Simulator;
import javacard.framework.AID;
public class MixedAPDUHarness {
 static Simulator sim;
 static byte[] send(int ins,int p2,byte[] data){byte[] cmd=new byte[data.length==0?4:5+data.length];cmd[0]=(byte)0xB0;cmd[1]=(byte)ins;cmd[3]=(byte)p2;if(data.length>0){cmd[4]=(byte)data.length;System.arraycopy(data,0,cmd,5,data.length);}byte[] r=sim.transmitCommand(cmd);if(r[r.length-2]!=(byte)0x90||r[r.length-1]!=0)throw new AssertionError("mixed APDU status "+ins);return Arrays.copyOf(r,r.length-2);}
 static byte[] close(byte[] data)throws Exception{byte[] b=new byte[34];b[0]=(byte)(data.length>>8);b[1]=(byte)data.length;System.arraycopy(java.security.MessageDigest.getInstance("SHA-256").digest(data),0,b,2,32);return b;}
 public static void main(String[] args)throws Exception{
  sim=new Simulator();AID aid=new AID(new byte[]{(byte)0xF0,0,0,1,2,1},(short)0,(byte)6);sim.installApplet(aid,StreamDemoApplet.class);sim.selectApplet(aid);
  byte[] identity=send(2,0,new byte[0]);if(identity.length!=WIDTH)throw new AssertionError("mixed APDU exact width");for(int i=0;i<identity.length;i++)if(identity[i]!=(byte)i)throw new AssertionError("mixed APDU fixed wire");
  byte[] input={1,2,3};send(0x20,1,input);send(0x21,0,close(input));if(!Arrays.equals(send(0x23,1,new byte[0]),new byte[]{3,2,1}))throw new AssertionError("mixed APDU close wire");send(0x24,0,close(new byte[]{3,2,1}));
  send(0x30,0,new byte[0]);if(!Arrays.equals(send(0x33,1,new byte[0]),new byte[]{1,2,3,1}))throw new AssertionError("mixed APDU response-only wire");send(0x35,0,new byte[0]);
 }
}
`
