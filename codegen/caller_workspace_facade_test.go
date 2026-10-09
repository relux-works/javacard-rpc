package codegen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The actual Counter process entry lends the whole current APDU window, while
// retaining independent exact reply capacity. Four callback shapes check its
// geometry and write a tail sentinel observed on that same APDU after dispatch.
// Direct invalid/empty spans exercise the released dispatcher before effects.
// Reflection substitutes only the test observer; no production array is retained.
func TestCounterCallerWorkspaceContract(t *testing.T) {
	runCounterWriterHarnessSource(t, counterCallerWorkspaceHarness(), "", "")
}

// These plants preserve dispatchTo and the borrowed buffer token but narrow
// authority to output capacity, shift the window, or substitute a fresh array.
// Each must fail the behavioral production-entry test, never javac.
func TestCounterCallerWorkspaceNarrowingMutants(t *testing.T) {
	p := filepath.Join("../examples/counter/applet/src/main/java/io/jcrpc/example", "CounterJCApplet.java")
	raw, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	before := "buffer, (short) 0, (short) buffer.length);"
	for _, tc := range []struct{ name, after, assertion string }{
		{"output-capacity-as-scratch", "buffer, (short) 0, outputCapacity);", "caller workspace geometry"},
		{"shift-caller-window", "buffer, (short) 1, (short) (buffer.length - 1));", "caller workspace geometry"},
		{"substitute-caller-array", "new byte[buffer.length], (short) 0, (short) buffer.length);", "caller workspace identity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if strings.Count(string(raw), before) != 1 {
				t.Fatal("workspace plant not unique")
			}
			source := strings.Replace(string(raw), before, tc.after, 1)
			runCounterWriterHarnessSource(t, counterCallerWorkspaceHarness(), "CounterJCApplet.java", source, tc.assertion)
			t.Log("named failing test: TestCounterCallerWorkspaceContract; real Java child exit 1")
		})
	}
	t.Run("retain-workspace-on-get", func(t *testing.T) {
		b, e := os.ReadFile(filepath.Join(filepath.Dir(p), "CounterApplet.java"))
		if e != nil {
			t.Fatal(e)
		}
		source := strings.Replace(string(b), "private short counter;", "private byte[] retainedWorkspace;\n    private short counter;", 1)
		before := "protected short onGet(byte[] callerWorkspace, short callerWorkspaceOffset, short callerWorkspaceCapacity) {\n        return counter;"
		if strings.Count(source, before) != 1 {
			t.Fatal("retention plant not unique")
		}
		source = strings.Replace(source, before, strings.Replace(before, "return counter;", "retainedWorkspace=callerWorkspace; return counter;", 1), 1)
		runCounterWriterHarnessSource(t, counterCallerWorkspaceHarness(), "CounterApplet.java", source, "borrowed workspace retained")
	})
}

func counterCallerWorkspaceHarness() string {
	s := strings.Replace(counterWriterHarness, "public static void main(String[] args){", "public static void main(String[] args) throws Exception{", 1)
	s = strings.Replace(s, "malformedReceive(); directSpans();", "malformedReceive(); directSpans(); callerWorkspace();", 1)
	return strings.TrimSuffix(s, "}") + counterCallerWorkspaceMethods + "}\n"
}

const counterCallerWorkspaceMethods = `
 static class WorkspaceTracking extends CounterApplet {
  int calls; short expectedCapacity,expectedOffset;
  void observe(byte[] w,short off,short cap){
   check(off==expectedOffset&&cap==expectedCapacity,"caller workspace geometry");
   calls++;
   if(cap>0)w[(short)(off+cap-1)]=42;
  }
  protected short onGet(byte[] w,short off,short cap){observe(w,off,cap);return super.onGet(w,off,cap);}
  protected void onReset(byte[] w,short off,short cap){observe(w,off,cap);super.onReset(w,off,cap);}
  protected short onGetInfo(byte[] out,short at,short count,byte[] w,short off,short cap){
   check(count==7,"independent exact reply capacity");observe(w,off,cap);return super.onGetInfo(out,at,count,w,off,cap);
  }
  protected short onEchoMessage(byte[] in,short at,short len,byte[] out,short outAt,short count,byte[] w,short off,short cap){
   observe(w,off,cap);return super.onEchoMessage(in,at,len,out,outAt,count,w,off,cap);
  }
 }
 static void callerWorkspace() throws Exception {
  WorkspaceTracking logic=new WorkspaceTracking();
  java.lang.reflect.Field f=CounterJCApplet.class.getDeclaredField("logic");f.setAccessible(true);f.set(app,logic);
  for(int capacity:new int[]{133,300}){
   logic.expectedCapacity=(short)capacity;
   for(int ins:new int[]{3,4,6,14}){
    byte[] input=ins==14?new byte[]{1,2,3,4}:empty;
    APDU a=frame(ins,0,input,input.length,5,capacity,input.length);int before=logic.calls;app.process(a);
    check(logic.calls==before+1,"workspace callback reachability");
    check(a.buffer[capacity-1]==42,"caller workspace identity");
    noRetention(logic,a.buffer);
    if(ins==6)check(a.sent.length==7,"workspace does not widen reply");
    if(ins==14)check(Arrays.equals(a.sent,input),"input/output last consumer");
   }
  }
  byte[] out=new byte[32],scratch=new byte[300];Arrays.fill(out,(byte)85);Arrays.fill(scratch,(byte)86);
  for(int kind=0;kind<7;kind++){
   byte[] w=kind==0?null:scratch;
   short off=kind==1?(short)-1:kind==3?(short)301:kind==4?(short)300:kind==5?(short)32767:(short)0;
   short cap=kind==2?(short)-1:kind==4?(short)1:kind==5?(short)32767:kind==6?(short)301:(short)0;
   int before=logic.calls;
   try{logic.dispatchTo((byte)6,(byte)0,(byte)0,null,(short)0,(short)0,out,(short)7,(short)7,w,off,cap);throw new AssertionError("invalid caller workspace admitted");}
   catch(CounterSkeleton.StatusWordException e){check((e.getStatusWord()&65535)==0x6700,"workspace exact refusal");}
   check(logic.calls==before,"workspace forbidden callback effects");
   for(byte b:out)check(b==85,"workspace rejection partial output");
   for(byte b:scratch)check(b==86,"workspace rejection scratch write");
  }
  // The framework accepts an empty end window; unused scalar/void need no RAM.
  logic.expectedCapacity=0;
  check(logic.dispatchTo((byte)3,(byte)0,(byte)0,null,(short)0,(short)0,out,(short)7,(short)2,empty,(short)0,(short)0)==2,"empty workspace valid control");
  // Nonzero disjoint scratch geometry is passed exactly by the dispatcher.
  logic.expectedCapacity=1;logic.expectedOffset=299;
  check(logic.dispatchTo((byte)6,(byte)0,(byte)0,null,(short)0,(short)0,out,(short)7,(short)7,scratch,(short)299,(short)1)==7,"contained boundary control");
  check(scratch[298]==86&&scratch[299]==42,"nonzero scratch exact plumbing");noRetention(logic,scratch);
  check(out[6]==85&&out[14]==85,"scratch independent output neighbors");
 }
 static void noRetention(Object logic,byte[] borrowed) throws Exception {
  for(Class<?> c=logic.getClass();c!=null;c=c.getSuperclass())for(java.lang.reflect.Field f:c.getDeclaredFields()){
   f.setAccessible(true);Object value=f.get(logic);check(value!=borrowed,"borrowed workspace retained");
   if(value instanceof Object[])for(Object slot:(Object[])value)check(slot!=borrowed,"borrowed workspace retained in slot");
  }
 }
`
