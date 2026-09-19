package io.jcrpc.counter.server;

import java.lang.reflect.Field;
import java.lang.reflect.Modifier;
import javacard.framework.JCSystem;

/**
 * JVM regression witness for security audit S-01 (javacard-rpc, plan row T-20).
 *
 * Drives the generated production entry point CounterSkeleton.dispatch(...) and
 * proves, by object identity, that the generated short-dispatch default branch
 * and the generated helper error paths never allocate an exception per call:
 * every failure raised by generated code is the one instance the skeleton
 * constructed at install time. A Java Card heap is never reclaimed, so
 * "same object every time" is the JVM-observable form of "zero new objects".
 *
 * Positive control: a developer-side {@code new StatusWordException} in the
 * abstract handler is a distinct object, so the identity assertion is
 * discriminating and not satisfied by a vacuous comparison.
 */
public final class DispatchAllocationHarness {
	private static final int UNKNOWN_FRAMES = 10_000;
	private static final int ALTERNATING_STATUS_FRAMES = 10_000;

    private static final byte INS_INCREMENT = (byte) 0x01;
    private static final byte INS_GET = (byte) 0x03;
    private static final byte INS_SET_LIMIT = (byte) 0x05;
    private static final byte INS_SET_ENABLED = (byte) 0x0A;
    private static final byte INS_GET_INFO = (byte) 0x06;
    private static final byte INS_GET_HASH = (byte) 0x0B;

    private static final short SW_WRONG_LENGTH = (short) 0x6700;
    private static final short SW_INS_NOT_SUPPORTED = (short) 0x6D00;
    private static final short SW_BUSINESS = (short) 0x6A86;

    public static void main(String[] args) throws Exception {
        Logic logic = new Logic();

		testAlternatingStatusesKeepTheSameExceptionAndTransientFields();

        // 1. N unknown-INS frames: every throw is the same object and keeps 6D00.
        CounterSkeleton.StatusWordException first = expectFailure(logic, (byte) 0x40, null);
        require(first.getStatusWord() == SW_INS_NOT_SUPPORTED, "unknown INS must answer 6D00");
        for (int frame = 1; frame < UNKNOWN_FRAMES; frame++) {
            // Sweep 0x40..0x7F: none of these is a declared instruction.
            byte ins = (byte) (0x40 + (frame & 0x3F));
            CounterSkeleton.StatusWordException failure = expectFailure(logic, ins, null);
            require(failure == first, "unknown INS frame " + frame + " allocated a new exception");
            require(failure.getStatusWord() == SW_INS_NOT_SUPPORTED,
                    "unknown INS frame " + frame + " changed the status word");
        }

        // 2. A valid request still succeeds afterwards (the applet is not wedged).
        byte[] response = logic.dispatch(INS_INCREMENT, (byte) 5, (byte) 0, null);
        require(response.length == 2 && response[0] == 0 && response[1] == 5,
                "increment after the unknown-INS loop must return 0x0005");
        require(logic.increments == 1, "handler must run exactly once");

        // 3. Generated helper error paths reuse the same instance:
        //    a) data request shorter than its fixed prefix -> length guard.
        CounterSkeleton.StatusWordException shortRequest = expectFailure(logic, INS_SET_LIMIT, new byte[]{1});
        require(shortRequest == first, "short request allocated a new exception");
        require(shortRequest.getStatusWord() == SW_WRONG_LENGTH, "short request must answer 6700");
        //    b) invalid bool encoding in P1 -> readBool guard.
        CounterSkeleton.StatusWordException badBool = expectFailure(logic, INS_SET_ENABLED, (byte) 2, null);
        require(badBool == first, "invalid bool allocated a new exception");
        require(badBool.getStatusWord() == SW_WRONG_LENGTH, "invalid bool must answer 6700");
        //    c) fixed packed response of the wrong length -> response guard.
        logic.infoLength = 4;
        CounterSkeleton.StatusWordException badInfo = expectFailure(logic, INS_GET_INFO, null);
        require(badInfo == first, "wrong packed response length allocated a new exception");
        require(badInfo.getStatusWord() == SW_WRONG_LENGTH, "wrong packed response length must answer 6700");
        //    d) fixed byte-sequence response of the wrong length -> packBytes guard.
        logic.hashLength = 31;
        CounterSkeleton.StatusWordException badHash = expectFailure(logic, INS_GET_HASH, null);
        require(badHash == first, "wrong fixed bytes response allocated a new exception");
        require(badHash.getStatusWord() == SW_WRONG_LENGTH, "wrong fixed bytes response must answer 6700");

        // 4. Still healthy after the helper failures.
        logic.infoLength = 5;
        require(logic.dispatch(INS_GET_INFO, (byte) 0, (byte) 0, null).length == 5, "getInfo must recover");
        require(logic.dispatch(INS_GET, (byte) 0, (byte) 0, null).length == 2, "get must recover");

        // 5. Positive control: a developer-thrown business exception is a different object.
        logic.failIncrement = true;
        CounterSkeleton.StatusWordException business = expectFailure(logic, INS_INCREMENT, (byte) 1, null);
        require(business != first, "positive control: developer exception must be a distinct object");
        require(business.getStatusWord() == SW_BUSINESS, "positive control status word");
        //    and the shared instance is not disturbed by it.
        CounterSkeleton.StatusWordException again = expectFailure(logic, (byte) 0x41, null);
        require(again == first && again.getStatusWord() == SW_INS_NOT_SUPPORTED,
                "shared instance must survive a developer exception");

        // 6. Two skeleton instances own two distinct preconstructed exceptions
        //    (no static sharing across applet instances).
        Logic other = new Logic();
        require(expectFailure(other, (byte) 0x40, null) != first,
                "each skeleton instance must own its exception");
    }

	// R3-04 regression: alternating 6D00/6700 frames must reuse the same
	// exception object while only changing its CLEAR_ON_RESET array contents.
	// Reflection snapshots the object's own instance fields, so a persistent
	// short status field fails on the first transition even though object identity
	// remains stable.
	private static void testAlternatingStatusesKeepTheSameExceptionAndTransientFields() throws Exception {
		Logic logic = new Logic();
		Field[] fields = instanceFields(CounterSkeleton.StatusWordException.class);
		Field statusBacking = null;
		for (Field field : fields) {
			if (field.getType() == short[].class) {
				statusBacking = field;
				break;
			}
		}
		require(statusBacking != null,
				"reusable status exception must own a transient short[] backing store");

		CounterSkeleton.StatusWordException first = null;
		Object[] initialFields = null;
		for (int frame = 0; frame < ALTERNATING_STATUS_FRAMES; frame++) {
			boolean unknownInstruction = (frame & 1) == 0;
			CounterSkeleton.StatusWordException failure = unknownInstruction
					? expectFailure(logic, (byte) 0x40, null)
					: expectFailure(logic, INS_SET_LIMIT, new byte[]{1});
			short expected = unknownInstruction ? SW_INS_NOT_SUPPORTED : SW_WRONG_LENGTH;

			if (first == null) {
				first = failure;
				initialFields = snapshot(fields, failure);
				Object backing = statusBacking.get(failure);
				require(backing instanceof short[],
						"status backing field must remain an array reference");
				require(JCSystem.isTransientForTest(backing),
						"status backing field must reference the registered transient array");
			} else {
				require(failure == first,
						"alternating frame " + frame + " allocated a different exception");
				requireFieldsUnchanged(fields, initialFields, failure, frame);
			}
			require(failure.getStatusWord() == expected,
					"alternating frame " + frame + " returned the wrong status word");
		}
	}

	private static Field[] instanceFields(Class<?> type) {
		Field[] declared = type.getDeclaredFields();
		int count = 0;
		for (Field field : declared) {
			if (!Modifier.isStatic(field.getModifiers())) count++;
		}
		Field[] result = new Field[count];
		int index = 0;
		for (Field field : declared) {
			if (Modifier.isStatic(field.getModifiers())) continue;
			field.setAccessible(true);
			result[index++] = field;
		}
		return result;
	}

	private static Object[] snapshot(Field[] fields, Object target) throws IllegalAccessException {
		Object[] result = new Object[fields.length];
		for (int index = 0; index < fields.length; index++) {
			result[index] = fields[index].get(target);
		}
		return result;
	}

	private static void requireFieldsUnchanged(Field[] fields, Object[] initial,
			Object target, int frame) throws IllegalAccessException {
		for (int index = 0; index < fields.length; index++) {
			Object before = initial[index];
			Object after = fields[index].get(target);
			boolean unchanged = fields[index].getType().isPrimitive()
					? before.equals(after)
					: before == after;
			require(unchanged,
					"persistent field " + fields[index].getName()
							+ " changed on alternating frame " + frame);
		}
	}

    private static CounterSkeleton.StatusWordException expectFailure(Logic logic, byte ins, byte[] data) {
        return expectFailure(logic, ins, (byte) 0, data);
    }

    private static CounterSkeleton.StatusWordException expectFailure(Logic logic, byte ins, byte p1, byte[] data) {
        try {
            logic.dispatch(ins, p1, (byte) 0, data);
        } catch (CounterSkeleton.StatusWordException failure) {
            return failure;
        }
        throw new AssertionError(String.format("INS %02X was accepted", ins & 0xFF));
    }

    private static void require(boolean condition, String message) {
        if (!condition) {
            throw new AssertionError(message);
        }
    }

    private static final class Logic extends CounterSkeleton {
        int increments;
        int infoLength = 5;
        int hashLength = 32;
        boolean failIncrement;
        private short value;

        Logic() {
            super(new CounterTransport() {
                public byte[] transmit(byte ins, byte p1, byte p2, byte[] data) {
                    return new byte[0];
                }
            });
        }

        protected short onIncrement(byte amount) {
            if (failIncrement) {
                throw new StatusWordException(SW_BUSINESS);
            }
            increments++;
            value = (short) (value + (amount & 0xFF));
            return value;
        }

        protected short onDecrement(byte amount) {
            value = (short) (value - (amount & 0xFF));
            return value;
        }

        protected short onGet() {
            return value;
        }

        protected void onReset() {
            value = 0;
        }

        protected void onSetLimit(short limit) {
        }

        protected byte[] onGetInfo() {
            return new byte[infoLength];
        }

        protected void onStore(byte[] data) {
        }

        protected byte[] onLoad() {
            return new byte[0];
        }

        protected void onSetCount(int newValue) {
            value = (short) newValue;
        }

        protected void onSetEnabled(boolean enabled) {
        }

        protected byte[] onGetHash() {
            return new byte[hashLength];
        }
    }
}
