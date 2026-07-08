---
title: JavaCard RPC
summary: An RPC framework for Java Card smart-card applets, IDL to typed clients.
category: Architecture libraries
featured: true
---

## What it is

An RPC framework for Java Card smart-card applets. You describe the interface once in a
TOML IDL, and codegen produces the on-card server skeleton plus typed client runtimes —
in Kotlin/JVM, Swift, and Java — that speak the APDU protocol for you. No hand-rolling
command and response byte layouts.

## Why it matters

Smart-card development is famously low-level: every call is raw APDU bytes with manual
serialization. Bringing an IDL-plus-codegen workflow to Java Card turns that into
ordinary typed method calls across languages, which is the kind of breadth that shows
where our stack really reaches — from AI agents down to secure elements.

## Who it is for

Engineers building on secure elements and smart cards who want a modern, type-safe
client/server workflow instead of byte wrangling.
