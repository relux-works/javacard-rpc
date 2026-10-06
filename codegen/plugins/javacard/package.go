package javacard

import (
	"fmt"
	"strings"
)

func GenerateBuildGradle(javaPackage, version string, _ bool, simulatorDependency string) string {
	// extract group from package: io.jcrpc.counter.server -> io.jcrpc
	group := javaPackageGroup(javaPackage)
	// Every generated skeleton uses JCSystem for CLEAR_ON_RESET status storage.
	// Keep the compile-only Java Card API available for ordinary and streamed
	// packages alike.
	dependencies := fmt.Sprintf(`
dependencies {
    compileOnly '%s'
}
`, simulatorDependency)
	return fmt.Sprintf(`plugins {
    id 'java-library'
}

group = '%s'
version = '%s'

java {
    sourceCompatibility = JavaVersion.VERSION_1_8
    targetCompatibility = JavaVersion.VERSION_1_8
}

tasks.withType(JavaCompile).configureEach {
    options.compilerArgs += ['-Xlint:-options']
}

repositories {
    mavenCentral()
    mavenLocal()
}
%s`, group, version, dependencies)
}

func javaPackageGroup(pkg string) string {
	parts := strings.Split(pkg, ".")
	if len(parts) <= 2 {
		return pkg
	}
	return strings.Join(parts[:2], ".")
}
