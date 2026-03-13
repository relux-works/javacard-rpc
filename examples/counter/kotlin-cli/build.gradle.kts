plugins {
    application
    kotlin("jvm") version "2.1.10"
}

group = "io.jcrpc.counter"
version = "0.1.0"

kotlin {
    jvmToolchain(17)
}

dependencies {
    implementation("counter:counter-client-kotlin")
    implementation("io.jcrpc:javacard-rpc-client-kotlin")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.10.2")
}

application {
    mainClass = "io.jcrpc.example.CounterE2EKt"
}
