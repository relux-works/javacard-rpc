package io.jcrpc.bridge;

import io.jcrpc.bridge.card.CardProvider;
import com.licel.jcardsim.smartcardio.CardSimulator;

import javax.smartcardio.CommandAPDU;
import javax.smartcardio.ResponseAPDU;
import java.util.concurrent.CountDownLatch;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Card whose transmitCommand parks the first caller until released and counts
 * how many callers are inside at once. Lets a test prove the shared-scope lock
 * deterministically instead of hoping a data race shows up.
 */
public class BlockingCardProvider implements CardProvider {
    public final CountDownLatch firstEntered = new CountDownLatch(1);
    public final CountDownLatch release = new CountDownLatch(1);
    public final AtomicInteger inside = new AtomicInteger();
    public final AtomicInteger maxInside = new AtomicInteger();
    public final AtomicInteger calls = new AtomicInteger();

    @Override
    public CardSimulator create() {
        return new CardSimulator() {
            @Override
            public ResponseAPDU transmitCommand(CommandAPDU cmd) {
                int now = inside.incrementAndGet();
                maxInside.accumulateAndGet(now, Math::max);
                try {
                    if (calls.getAndIncrement() == 0) {
                        firstEntered.countDown();
                        try { release.await(30, TimeUnit.SECONDS); } catch (InterruptedException ignored) {}
                    }
                    return new ResponseAPDU(new byte[]{(byte) 0x90, 0x00});
                } finally {
                    inside.decrementAndGet();
                }
            }
        };
    }
}
