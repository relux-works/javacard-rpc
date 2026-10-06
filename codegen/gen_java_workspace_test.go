package codegen

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Exhaustive CLA vectors drive the generated production adapter, with explicit
// Oracle-derived allow lists independent of the generator's mask calculation.
// This verifies decoding, not physical MANAGE CHANNEL or applet selection.
func TestGeneratedStreamAdapterChannelCoding(t *testing.T) {
	for _, tc := range []struct {
		base           byte
		first, further []byte
	}{
		{0xB6, []byte{0xB4, 0xB5, 0xB6, 0xB7}, claRange(0xF0)[:15]},
		{0xB4, []byte{0xB4, 0xB5, 0xB6, 0xB7}, claRange(0xF0)[:15]},
		{0xB0, []byte{0xB0, 0xB1, 0xB2, 0xB3}, claRange(0xD0)},
		{0x80, []byte{0x80, 0x81, 0x82, 0x83}, claRange(0xC0)},
		{0x01, []byte{0, 1, 2, 3}, claRange(0x40)},
		{0x0C, []byte{0x0C, 0x0D, 0x0E, 0x0F}, claRange(0x60)},
		{0xD0, []byte{0x90, 0x91, 0x92, 0x93}, claRange(0xD0)},
		{0xE0, []byte{0x8C, 0x8D, 0x8E, 0x8F}, claRange(0xE0)},
		{0xF0, []byte{0x9C, 0x9D, 0x9E, 0x9F}, claRange(0xF0)[:15]},
		{0xFF, nil, nil},
		{0x21, []byte{0x21}, nil},
	} {
		t.Run(fmt.Sprintf("%02X", tc.base), func(t *testing.T) {
			s := workspaceSchema(t, "")
			s.Applet.CLA = tc.base
			result, err := GenerateJavaSkeleton(s, "io.jcrpc.streamdemo.server")
			if err != nil {
				t.Fatal(err)
			}
			allowed := append(tc.first, tc.further...)
			vectors := make([]string, len(allowed))
			for i, v := range allowed {
				vectors[i] = fmt.Sprintf("0x%02X", v)
			}
			harness := strings.ReplaceAll(channelHarness, "ALLOWED", strings.Join(vectors, ","))
			if output, err := runWorkspaceJava(t, result, harness, apduJCSystemStub); err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
		})
	}
}
func claRange(first byte) []byte {
	v := make([]byte, 16)
	for i := range v {
		v[i] = first + byte(i)
	}
	return v
}
func workspaceSchema(t *testing.T, policy string) *Schema {
	t.Helper()
	s, e := ParseFile("testdata/stream.toml")
	if e != nil {
		t.Fatal(e)
	}
	s.Applet.StreamWorkspace = policy
	return s
}

// Both lifecycle modes exercise the same wire transcript, retry integrity,
// explicit abort/deselect wipe, and reset invalidation through processIfStream.
// The tracked Java Card stub clears transient arrays, never persistent bytes;
// real simulator/CAP checks are a separate lane.
func TestGeneratedPersistentWorkspaceLifecycle(t *testing.T) {
	for _, memory := range []StreamMemory{StreamMemoryClearOnDeselect, StreamMemoryClearOnReset} {
		for _, policy := range []string{"transient", "persistent"} {
			t.Run(policy+"/"+string(memory), func(t *testing.T) {
				s := workspaceSchema(t, policy)
				result, e := GenerateJavaSkeletonWithOptions(s, "io.jcrpc.streamdemo.server", JavaOptions{StreamMemory: memory})
				if e != nil {
					t.Fatal(e)
				}
				harness := strings.ReplaceAll(workspaceLifecycleHarness, "PERSISTENT", fmt.Sprint(policy == "persistent"))
				if output, e := runWorkspaceJava(t, result, harness, trackedJCSystem); e != nil {
					t.Fatalf("%v\n%s", e, output)
				}
				// Reuse the existing fragmented and wire transcript against both policies.
				harness = strings.ReplaceAll(streamAPDUAdapterHarness, "StreamAPDUAdapterHarness", "WorkspaceHarness")
				if output, e := runWorkspaceJava(t, result, harness, trackedJCSystem); e != nil {
					t.Fatalf("existing transcript: %v\n%s", e, output)
				}
			})
		}
	}
}

// Every planted mutation keeps the gate and admits a bounded forbidden case.
// A kill requires the expected behavioral assertion; compilation/setup failure
// cannot count. The wipe mutation preserves the searched-for wipe token.
func TestGeneratedWorkspaceNarrowingMutants(t *testing.T) {
	s := workspaceSchema(t, "persistent")
	s.Applet.CLA = 0xB6
	original, e := GenerateJavaSkeleton(s, "io.jcrpc.streamdemo.server")
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name, from, to, assertion string
		adapter                   bool
	}{
		{"wrong-security-first", "(cla & 0xFC) == 0xB4", "cla == (byte) 0xB0 || (cla & 0xFC) == 0xB4", "CLA rejection", true},
		{"wrong-chaining-further", "(cla & 0xF0) == 0xF0", "cla == (byte) 0xE0 || (cla & 0xF0) == 0xF0", "CLA rejection", true},
		{"wrong-class-first", "(cla & 0xFC) == 0xB4", "cla == (byte) 0x34 || (cla & 0xFC) == 0xB4", "CLA rejection", true},
		{"persistent-wipe-prefix-nine", "wipe(workspace);", "if (workspace[0] != (byte) 9) wipe(workspace);", "workspace wipe", false},
		{"reset-cleanup-prefix-nine", "if (resetMarker[0] == 0)", "if (resetMarker[0] == 0 && workspace[0] != (byte) 9)", "workspace wipe", false},
		{"retry-integrity-prefix-nine", "!equalsRange(workspace, scalars[IDX_LAST_CHUNK_OFFSET],", "request[requestOffset] != (byte) 9 && !equalsRange(workspace, scalars[IDX_LAST_CHUNK_OFFSET],", "expected refusal: 27264", false},
		{"close-digest-prefix-nine", "!equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)", "workspace[0] != (byte) 9 && !equalsRange(digestScratch, (short) 0, request, digestOffset, DIGEST_LENGTH)", "expected refusal: 27264", false},
		{"per-command-transient-prefix-one", "validateRange(requestBuffer, requestOffset, requestLength);", "if (requestLength == 1 && requestBuffer[requestOffset] == (byte) 1) javacard.framework.JCSystem.makeTransientByteArray((short) 1, javacard.framework.JCSystem.CLEAR_ON_RESET);\n            validateRange(requestBuffer, requestOffset, requestLength);", "per-command transient allocation", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := *original
			harness := strings.ReplaceAll(workspaceLifecycleHarness, "PERSISTENT", "true")
			if tc.adapter {
				result.StreamAPDUAdapterSource = []byte(strings.Replace(string(result.StreamAPDUAdapterSource), tc.from, tc.to, 1))
				harness = strings.ReplaceAll(channelHarness, "ALLOWED", "0xB4,0xB5,0xB6,0xB7,0xF0,0xF1,0xF2,0xF3,0xF4,0xF5,0xF6,0xF7,0xF8,0xF9,0xFA,0xFB,0xFC,0xFD,0xFE")
			} else {
				result.StreamRuntimeSource = []byte(strings.ReplaceAll(string(result.StreamRuntimeSource), tc.from, tc.to))
				// Lifecycle uses B0 vectors; use its matching adapter unchanged.
				lifecycle, e := GenerateJavaSkeleton(workspaceSchema(t, "persistent"), "io.jcrpc.streamdemo.server")
				if e != nil {
					t.Fatal(e)
				}
				result.StreamAPDUAdapterSource = lifecycle.StreamAPDUAdapterSource
			}
			output, e := runWorkspaceJava(t, &result, harness, trackedJCSystem)
			if e == nil || !strings.Contains(output, "AssertionError: "+tc.assertion) {
				t.Fatalf("mutant not killed by expected assertion: %v\n%s", e, output)
			}
			t.Logf("killed %s: %s", tc.name, tc.assertion)
		})
	}
}

// Policy parsing and validation have valid controls and reject unknown values.
// Explicit transient leaves every generated byte identical to the default.
func TestStreamWorkspacePolicy(t *testing.T) {
	for _, policy := range []string{"", "transient", "persistent", "ram", "Persistent"} {
		input, e := os.ReadFile("testdata/stream.toml")
		if e != nil {
			t.Fatal(e)
		}
		input = bytes.Replace(input, []byte("[applet]"), []byte("[applet]\nstream_workspace = \""+policy+"\""), 1)
		s, e := Parse(bytes.NewReader(input))
		if e != nil {
			t.Fatal(e)
		}
		errors := Validate(s)
		valid := policy == "" || policy == "transient" || policy == "persistent"
		result, e := GenerateJavaSkeleton(s, "io.jcrpc.streamdemo.server")
		if valid && (len(errors) != 0 || e != nil) {
			t.Fatalf("valid %q: %v %v", policy, errors, e)
		}
		if !valid && (len(errors) != 1 || errors[0].Path != "applet.stream_workspace" || e == nil || result != nil) {
			t.Fatalf("invalid %q: %v %v", policy, errors, e)
		}
	}
	a, _ := GenerateJavaSkeleton(workspaceSchema(t, ""), "io.jcrpc.streamdemo.server")
	b, _ := GenerateJavaSkeleton(workspaceSchema(t, "transient"), "io.jcrpc.streamdemo.server")
	if !reflect.DeepEqual(a, b) {
		t.Fatal("explicit transient changed default output")
	}
}

func runWorkspaceJava(t *testing.T, r *JavaGenerationResult, harness, jcsystem string) (string, error) {
	t.Helper()
	root := t.TempDir()
	pkg := "io/jcrpc/streamdemo/server/"
	files := map[string][]byte{
		pkg + r.TransportName + ".java": r.TransportSource, pkg + r.SkeletonName + ".java": r.SkeletonSource,
		pkg + r.StreamEndpointName + ".java": r.StreamEndpointSource, pkg + r.StreamRuntimeName + ".java": r.StreamRuntimeSource,
		pkg + r.StreamAPDUAdapterName + ".java": r.StreamAPDUAdapterSource, pkg + "StreamDemoApplet.java": []byte(streamDemoAppletFixture),
		pkg + "WorkspaceHarness.java": []byte(harness), "javacard/framework/JCSystem.java": []byte(jcsystem),
		"javacard/framework/ISO7816.java": []byte(apduISO7816Stub), "javacard/framework/ISOException.java": []byte(apduISOExceptionStub),
		"javacard/framework/Applet.java": []byte(apduAppletStub), "javacard/framework/APDU.java": []byte(fragmentedAPDUStub),
		"javacard/security/MessageDigest.java": []byte(apduMessageDigestStub),
	}
	args := []string{"-d", root}
	for name, source := range files {
		path := filepath.Join(root, name)
		if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(path, source, 0644); e != nil {
			t.Fatal(e)
		}
		args = append(args, path)
	}
	if output, e := exec.Command("javac", args...).CombinedOutput(); e != nil {
		t.Fatalf("compile failed: %v\n%s", e, output)
	}
	output, e := exec.Command("java", "-cp", root, "io.jcrpc.streamdemo.server.WorkspaceHarness").CombinedOutput()
	return string(output), e
}

const channelHarness = `package io.jcrpc.streamdemo.server;
import javacard.framework.*;
public class WorkspaceHarness {
 public static void main(String[] args) {
  boolean[] allowed=new boolean[256];for(int cla:new int[]{ALLOWED})allowed[cla]=true;
  StreamDemoApplet applet=new StreamDemoApplet();
  // Invoke a response handler for every CLA. Rejected commands must not execute it.
  for(int cla=0;cla<256;cla++){
   int before=applet.responseOnlyCallsForTest()&255;
   APDU command=new APDU((byte)cla,(byte)0x30,(byte)0,(byte)0,new byte[0],new int[0]);
   try{applet.process(command);if(!allowed[cla])throw new AssertionError("CLA rejection: "+cla);}
   catch(ISOException e){if(allowed[cla] || e.sw!=(short)0x6E00)throw new AssertionError("CLA acceptance/status: "+cla);}
   if((applet.responseOnlyCallsForTest()&255)!=before+(allowed[cla]?1:0))throw new AssertionError("handler effects: "+cla);
   if(!allowed[cla] && command.outgoing.length!=0)throw new AssertionError("rejected output");
   applet.deselect();
   // Ordinary instructions must still return false before any CLA check.
   if(applet.streamsForTest().processIfStream(new APDU((byte)cla,(byte)1,(byte)0,(byte)0,new byte[0],new int[0])))throw new AssertionError("fallback");
  }
 }
}
`

const trackedJCSystem = `package javacard.framework;
import java.util.*;
public final class JCSystem {
 public static final byte CLEAR_ON_RESET=0,CLEAR_ON_DESELECT=1;
 public static final List<Object> arrays=new ArrayList<>();
 public static final List<Byte> events=new ArrayList<>();
 public static byte[] makeTransientByteArray(short n,byte event){byte[] a=new byte[n];arrays.add(a);events.add(event);return a;}
 public static short[] makeTransientShortArray(short n,byte event){short[] a=new short[n];arrays.add(a);events.add(event);return a;}
 public static Object[] makeTransientObjectArray(short n,byte event){Object[] a=new Object[n];arrays.add(a);events.add(event);return a;}
 public static void reset(){for(Object a:arrays){if(a instanceof byte[])Arrays.fill((byte[])a,(byte)0);else if(a instanceof short[])Arrays.fill((short[])a,(short)0);else Arrays.fill((Object[])a,null);}}
}
`

const workspaceLifecycleHarness = `package io.jcrpc.streamdemo.server;
import javacard.framework.*;
import java.util.*;
import java.lang.reflect.*;
import java.security.MessageDigest;
public class WorkspaceHarness {
 static StreamDemoApplet applet;
 static byte[] workspace;
 static int arrays;
 public static void main(String[] args)throws Exception{
  applet=new StreamDemoApplet();
  Object logic=field(applet,"logic");Object runtime=field(logic,"streamSession");workspace=(byte[])field(runtime,"workspace");
  if(JCSystem.arrays.contains(workspace)==PERSISTENT)throw new AssertionError("storage policy");
  arrays=JCSystem.arrays.size();
  // All non-workspace mutable arrays must remain transient.
  for(String n:new String[]{"digestScratch","scalars","handlerSlot"})if(!JCSystem.arrays.contains(field(runtime,n)))throw new AssertionError("control storage: "+n);
  byte[] head=new byte[192];Arrays.fill(head,(byte)9);
  send(0x20,0,2,head);send(0x20,0,2,head); // Identical retry accepted.
  byte[] changed=head.clone();changed[1]=8;
  reject(0x6A80,0x20,0,2,changed);zero(); // Changed retry must wipe failed session.
  reject(0x6985,0x22,0,0,new byte[0]);
  for(int cleanup=0;cleanup<3;cleanup++){
   send(0x20,0,2,head);
   if(workspace[0]!=9)throw new AssertionError("live workspace control");
   if(cleanup==0)send(0x25,0,0,new byte[0]);
   if(cleanup==1)applet.deselect();
   if(cleanup==2){JCSystem.reset();if(PERSISTENT && workspace[0]!=9)throw new AssertionError("persistent reset residue control");}
   reject(0x6985,0x22,0,0,new byte[0]);zero();
   reject(0x6985,0x23,0,1,new byte[0]);
   byte[] fresh={1,2,3};send(0x20,0,1,fresh);
   byte[] descriptor=send(0x21,0,0,close(fresh));
   if(descriptor.length!=35)throw new AssertionError("descriptor wire");
   if(!Arrays.equals(descriptor,send(0x22,0,0,new byte[0])))throw new AssertionError("recovery wire");
   byte[] expected={3,2,1};if(!Arrays.equals(expected,send(0x23,0,1,new byte[0])))throw new AssertionError("fresh result");
   send(0x24,0,0,close(expected));zero();
  }
  // Bad digest must be rejected, not invoke the response-only handler.
  send(0x20,0,1,new byte[]{9});byte[] bad=close(new byte[]{9});bad[2]^=1;
  reject(0x6A80,0x21,0,0,bad);zero();
  if(applet.responseOnlyCallsForTest()!=0)throw new AssertionError("forbidden handler effects");
  // Reset while a result is pending must invalidate recovery without replaying
  // its handler; the next valid invocation remains reachable.
  send(0x30,0,0,new byte[0]);
  reject(0x6985,0x30,0,0,new byte[0]);
  if(applet.responseOnlyCallsForTest()!=1)throw new AssertionError("pending replay effects");
  JCSystem.reset();
  reject(0x6985,0x32,0,0,new byte[0]);zero();
  reject(0x6985,0x33,0,1,new byte[0]);
  if(applet.responseOnlyCallsForTest()!=1)throw new AssertionError("stale reset effects");
  send(0x30,0,0,new byte[0]);
  if(applet.responseOnlyCallsForTest()!=2)throw new AssertionError("fresh invoke control");
  applet.deselect();zero();
  // A fresh write directly after reset also wipes old bytes outside its prefix.
  send(0x20,0,2,head);JCSystem.reset();send(0x20,0,1,new byte[]{1});
  for(int i=1;i<workspace.length;i++)if(workspace[i]!=0)throw new AssertionError("reset fresh-write residual bytes");
  send(0x25,0,0,new byte[0]);zero();
  if(JCSystem.arrays.size()!=arrays)throw new AssertionError("per-command transient allocation");
 }
 static Object field(Object x,String n)throws Exception{Class<?> c=x.getClass();while(c!=null){try{Field f=c.getDeclaredField(n);f.setAccessible(true);return f.get(x);}catch(NoSuchFieldException e){c=c.getSuperclass();}}throw new NoSuchFieldException(n);}
 static byte[] send(int ins,int p1,int p2,byte[] data){APDU a=new APDU((byte)0xB0,(byte)ins,(byte)p1,(byte)p2,data,data.length==0?new int[0]:new int[]{data.length});applet.process(a);return a.outgoing;}
 static void reject(int sw,int ins,int p1,int p2,byte[] data){try{send(ins,p1,p2,data);throw new AssertionError("expected refusal: "+sw);}catch(ISOException e){if((e.sw&65535)!=sw)throw new AssertionError("wrong refusal status: "+(e.sw&65535));}}
 static void zero(){for(byte b:workspace)if(b!=0)throw new AssertionError("workspace wipe: residual bytes");}
 static byte[] close(byte[] data)throws Exception{byte[] result=new byte[34];result[0]=(byte)(data.length>>8);result[1]=(byte)data.length;System.arraycopy(MessageDigest.getInstance("SHA-256").digest(data),0,result,2,32);return result;}
}
`
