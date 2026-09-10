package io.jcrpc.bridge.card;

import io.jcrpc.bridge.config.AppletConfig;

import java.lang.reflect.Constructor;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Modifier;
import java.util.ArrayList;
import java.util.List;
import java.util.ServiceConfigurationError;
import java.util.ServiceLoader;

/**
 * Resolves the CardProvider for a bridge start:
 *   1. --card-provider fqcn  → that class, public no-arg constructor
 *   2. otherwise ServiceLoader → exactly one registered provider
 *   3. otherwise DefaultCardProvider over the --config applets
 * Every failure is a typed CardProviderStartupException; nothing hangs or
 * degrades silently to the default.
 */
public final class CardProviderLoader {
    private CardProviderLoader() {}

    public static CardProvider resolve(String fqcn, List<AppletConfig> applets)
            throws CardProviderStartupException {
        if (fqcn != null) {
            return loadByName(fqcn);
        }
        List<CardProvider> discovered = discover();
        if (discovered.size() == 1) {
            return discovered.get(0);
        }
        if (discovered.size() > 1) {
            List<String> names = new ArrayList<>();
            for (CardProvider p : discovered) names.add(p.getClass().getName());
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_AMBIGUOUS,
                    "multiple CardProviders registered via ServiceLoader, pass --card-provider: " + names);
        }
        return new DefaultCardProvider(applets);
    }

    public static CardProvider loadByName(String fqcn) throws CardProviderStartupException {
        Class<?> clazz;
        try {
            clazz = Class.forName(fqcn);
        } catch (ClassNotFoundException | LinkageError e) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_NOT_FOUND,
                    "class '" + fqcn + "' not on the bridge classpath", e);
        }
        if (!CardProvider.class.isAssignableFrom(clazz)) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_NOT_A_CARD_PROVIDER,
                    "class '" + fqcn + "' does not implement " + CardProvider.class.getName());
        }
        Constructor<?> ctor;
        try {
            ctor = clazz.getConstructor();
        } catch (NoSuchMethodException e) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_NO_NOARG_CTOR,
                    "class '" + fqcn + "' has no public no-arg constructor", e);
        }
        if (Modifier.isAbstract(clazz.getModifiers())) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_INSTANTIATION_FAILED,
                    "class '" + fqcn + "' is abstract");
        }
        try {
            return (CardProvider) ctor.newInstance();
        } catch (InvocationTargetException e) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_INSTANTIATION_FAILED,
                    "constructor of '" + fqcn + "' threw: " + e.getCause(), e.getCause());
        } catch (ReflectiveOperationException | RuntimeException e) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_INSTANTIATION_FAILED,
                    "cannot instantiate '" + fqcn + "': " + e, e);
        }
    }

    private static List<CardProvider> discover() throws CardProviderStartupException {
        List<CardProvider> found = new ArrayList<>();
        try {
            for (CardProvider p : ServiceLoader.load(CardProvider.class)) {
                found.add(p);
            }
        } catch (ServiceConfigurationError e) {
            throw new CardProviderStartupException(
                    CardProviderStartupException.Reason.PROVIDER_INSTANTIATION_FAILED,
                    "ServiceLoader failed to instantiate a CardProvider: " + e.getMessage(), e);
        }
        return found;
    }
}
