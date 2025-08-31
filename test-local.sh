#!/bin/bash

# HTTP Mirror Local Testing Script
echo "🔧 HTTP Mirror Local Testing"
echo "============================"

# Build the binaries
echo "📦 Building binaries..."
go build -o bin/testserver ./cmd/testserver
go build -o bin/updater ./cmd/updater
go build -o bin/server ./cmd/server

# Create data directory
mkdir -p test-data

echo ""
echo "🎯 Manual Testing Steps:"
echo "1. Start the test server:     ./bin/testserver"
echo "2. In another terminal, run:  ./bin/updater -config test-config.json"
echo "3. Check the downloaded files: ls -la test-data/"
echo "4. Start the web server:      ./bin/server -config test-config.json"
echo "5. View in browser:           http://localhost:3000"
echo ""

# Cleanup function
cleanup() {
  echo ""
  echo "🛑 Stopping servers..."

  # Kill by process name (more reliable)
  pkill -f "./bin/testserver" 2>/dev/null && echo "   ✓ Test server stopped"
  pkill -f "./bin/server.*test-config" 2>/dev/null && echo "   ✓ Web server stopped"

  # Also try the PID method as backup
  if [ ! -z "$TEST_SERVER_PID" ]; then
    kill $TEST_SERVER_PID 2>/dev/null
  fi
  if [ ! -z "$WEB_SERVER_PID" ]; then
    kill $WEB_SERVER_PID 2>/dev/null
  fi

  echo "🏁 Cleanup complete!"
  exit 0
}

# Function to run all steps automatically
run_demo() {
  echo "🚀 Running automated demo..."

  # Set up signal handlers for cleanup
  trap cleanup SIGINT SIGTERM

  # Start test server in background
  echo "Starting test server on port 8080..."
  ./bin/testserver &
  TEST_SERVER_PID=$!
  sleep 2

  # Run updater
  echo "Running updater to mirror files..."
  ./bin/updater -config test-config.json

  echo "📁 Downloaded files:"
  find test-data -type f -exec ls -la {} \;
  echo ""

  # Start web server in background
  echo "Starting web server on port 3000..."
  ./bin/server -config test-config.json &
  WEB_SERVER_PID=$!
  sleep 2

  echo ""
  echo "✅ Demo complete!"
  echo "🌐 Test server (original) running at: http://localhost:8080"
  echo "🖥️  Mirror web server running at: http://localhost:3000"
  echo ""
  echo "📋 What to check:"
  echo "   1. Visit http://localhost:8080 (original test server)"
  echo "   2. Visit http://localhost:3000 (mirrored files with original URL in footer)"
  echo "   3. Compare file1.txt and file2.txt on both servers"
  echo ""
  echo "Press Enter to stop servers (or Ctrl+C)..."
  read

  # Manual cleanup if user presses Enter
  cleanup
}

# Check if user wants to run demo
if [ "$1" = "demo" ]; then
  run_demo
else
  echo "Add 'demo' argument to run automated demo: $0 demo"
fi

