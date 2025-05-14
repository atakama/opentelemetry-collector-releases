#!/bin/bash

# Makefile for building OpenTelemetry Collector with custom builder
# This script is intended to be run in a Windows environment with MINGW64 (git bash) or similar
# It uses the OpenTelemetry Collector Builder (ocb) to build the collector
# and then compiles the collector binary using Go.
# Ensure the script is run from the correct directory

set -e  # Stop on first error

echo "📦 Generation de code GOLANG dans : distributions/otelcol/_build"
make build DISTRIBUTIONS=otelcol OTELCOL_BUILDER=/c/Users/frederic1/bin/ocb.exe OTELCOL_BUILDER_ARGS=--skip-compilation
cd distributions/otelcol/_build
echo "🔧 Compilation du collecteur"
go build -gcflags="all=-N -l" -o otelcol.exe
cd ../../..
echo "✅ Build terminé avec succès."
