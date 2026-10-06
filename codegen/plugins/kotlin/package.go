package kotlin

import "fmt"

func GenerateKotlinBuildGradle(appletLower, packageName, version string) string {
	return fmt.Sprintf(`plugins {
    kotlin("jvm") version "2.1.10"
}

group = %q
version = %q

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation("io.jcrpc:javacard-rpc-client-kotlin:0.2.0")
    testImplementation(kotlin("test"))
}

tasks.test {
    useJUnitPlatform()
}
`, packageName, version)
}

func GenerateKotlinSettingsGradle(appletLower string) string {
	return fmt.Sprintf(`pluginManagement {
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

sourceControl {
    gitRepository(uri("https://github.com/relux-works/javacard-rpc-client-kotlin.git")) {
        producesModule("io.jcrpc:javacard-rpc-client-kotlin")
    }
}

rootProject.name = %q
`, appletLower+"-client-kotlin")
}
