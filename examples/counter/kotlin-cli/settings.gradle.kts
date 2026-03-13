pluginManagement {
    repositories {
        gradlePluginPortal()
        mavenCentral()
    }
}

plugins {
    id("org.gradle.toolchains.foojay-resolver-convention") version "1.0.0"
}

dependencyResolutionManagement {
    repositoriesMode.set(RepositoriesMode.FAIL_ON_PROJECT_REPOS)
    repositories {
        mavenCentral()
    }
}

rootProject.name = "counter-kotlin-cli"

val useLocalResolvedDeps = providers.gradleProperty("useLocalResolvedDeps")
    .orElse("true")
    .get()
    .toBoolean()

includeBuild("../generated/counter-client-kotlin") {
    dependencySubstitution {
        substitute(module("counter:counter-client-kotlin")).using(project(":"))
    }
}

if (useLocalResolvedDeps) {
    includeBuild("../../../../javacard-rpc-client-kotlin") {
        dependencySubstitution {
            substitute(module("io.jcrpc:javacard-rpc-client-kotlin")).using(project(":"))
        }
    }
} else {
    sourceControl {
        gitRepository(uri("https://github.com/relux-works/javacard-rpc-client-kotlin.git")) {
            producesModule("io.jcrpc:javacard-rpc-client-kotlin")
        }
    }
}
