#!/bin/bash

# Makefile for building OpenTelemetry Collector with custom builder
# This script is intended to be run in a Windows environment with MINGW64 (git bash) or similar
# It uses the OpenTelemetry Collector Builder (ocb) to build the collector
# and then compiles the collector binary using Go.
# Ensure the script is run from the correct directory

set -e  # Stop on first error

echo "📦 Generation de code GOLANG dans : distributions/otelcol-contrib/_build"
make build DISTRIBUTIONS=otelcol-contrib OTELCOL_BUILDER=/c/Users/frederic1/bin/ocb.exe OTELCOL_BUILDER_ARGS=--skip-compilation
cd distributions/otelcol-contrib/_build
echo "🔧 Compilation du collecteur"
go build -gcflags="all=-N -l" -o otelcol-contrib.exe
cd ../../..
echo "✅ Build terminé avec succès."
cp -R ./distributions/exporter/nudgehttpexporter/HTML/ ./distributions/otelcol-contrib/_build/HTML/
echo "✅ Copie du dossier HTML terminé avec succès."
