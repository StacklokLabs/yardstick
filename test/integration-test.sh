#!/bin/bash

# Integration test script for yardstick MCP server
# Tests SSE and streamable-http transports using client binary
set -e

echo "Running integration tests for yardstick MCP server..."

# Test 1: Build the image using task build-image
echo "🏗️ Building Docker image using task build-image..."
task build-image
if [ $? -eq 0 ]; then
    echo "✓ Docker image built successfully using task build-image"
else
    echo "✗ Failed to build Docker image using task build-image"
    exit 1
fi

# Build client binary
echo "🔧 Building client binary..."
task build > /dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "✓ Client binary built successfully"
else
    echo "✗ Failed to build client binary"
    exit 1
fi

# Get the image name from ko build output
IMAGE_NAME="ghcr.io/stackloklabs/yardstick:latest"

###################################################################
################## START - STDIO TRANSPORT TESTING ################
###################################################################
echo ""
echo "🖥️  ========== STDIO TRANSPORT TESTING ==========" 
echo "📡 Testing STDIO endpoint with client binary..."
if ./build/yardstick-client -transport stdio -command="docker" -action info run --rm -i $IMAGE_NAME --transport stdio; then
    echo "✓ STDIO client connection successful"
else
    echo "! STDIO client connection failed"
    exit 1
fi

echo "📋 Testing tool listing via STDIO..."
if ./build/yardstick-client -transport stdio -command="docker" -action list-tools run --rm -i $IMAGE_NAME --transport stdio; then
    echo "✓ STDIO tools listing successful"
else
    echo "! STDIO tools listing failed"
    exit 1
fi

echo "🔧 Testing tool calling via STDIO..."
if ./build/yardstick-client -transport stdio -command="docker" -action=call-tool -tool=echo -args='{"input":"hellomatey123"}' run --rm -i $IMAGE_NAME --transport stdio | grep -q "hellomatey123"; then
    echo "✓ STDIO tool call returned expected output: hellomatey123"
else
    echo "! STDIO tool call did not return expected output: hellomatey123"
    exit 1
fi
###################################################################
################## END - STDIO TRANSPORT TESTING ##################
###################################################################


###################################################################
################## START - SSE TRANSPORT TESTING ##################
###################################################################
echo ""
echo "🌊 ========== SSE TRANSPORT TESTING ==========" 
echo "📡 Testing SSE transport on port 8080..."
docker run --rm -d --name yardstick-sse-test -p 8080:8080 $IMAGE_NAME --transport sse --port 8080 > /dev/null 2>&1
sleep 3

# Check if container is running
if docker ps | grep -q yardstick-sse-test; then
    echo "✓ SSE transport container started successfully on port 8080"
    
    # Test SSE endpoint with client binary
    echo "🌊 Testing SSE endpoint with client binary..."
    if ./build/yardstick-client -transport sse -address localhost -port 8080 -action info; then
        echo "✓ SSE client connection successful"
    else
        echo "! SSE client connection failed"
        exit 1
    fi
    
    # Test listing tools via SSE
    echo "📋 Testing tool listing via SSE..."
    if ./build/yardstick-client -transport sse -address localhost -port 8080 -action list-tools; then
        echo "✓ SSE tools listing successful"
    else
        echo "! SSE tools listing failed"
        exit 1
    fi
    
    echo "🔧 Testing tool calling via SSE..."
    if ./build/yardstick-client -transport sse -address localhost -port 8080 -action=call-tool -tool=echo -args='{"input":"hellomatey123"}' | grep -q "hellomatey123"; then
        echo "✓ SSE tool call returned expected output: hellomatey123"
    else
        echo "! SSE tool call did not return expected output: hellomatey123"
        exit 1
    fi
else
    echo "✗ SSE transport container failed to start on port 8080"
    exit 1
fi

# Cleanup SSE container
docker rm -f yardstick-sse-test > /dev/null 2>&1
echo "✓ SSE container shut down successfully"
###################################################################
################## END - SSE TRANSPORT TESTING ####################
###################################################################

###################################################################
############# START - StreamableHTTP TRANSPORT TESTING ############
###################################################################
echo ""
echo "🌐 ========== STREAMABLE-HTTP TRANSPORT TESTING ==========" 
echo "📡 Testing streamable-http transport on port 8081..."
docker run --rm -d --name yardstick-http-test -p 8081:8081 $IMAGE_NAME --transport streamable-http --port 8081 > /dev/null 2>&1
sleep 3

# Check if container is running
if docker ps | grep -q yardstick-http-test; then
    echo "✓ Streamable HTTP transport container started successfully on port 8081"
    
    # Test streamable-http endpoint with client binary
    echo "🌐 Testing streamable-http endpoint with client binary..."
    if ./build/yardstick-client -transport streamable-http -address localhost -port 8081 -action info; then
        echo "✓ Streamable HTTP client connection successful"
    else
        echo "! Streamable HTTP client connection failed"
        exit 1
    fi
    
    # Test listing tools via streamable-http
    echo "📋 Testing tool listing via streamable-http..."
    if ./build/yardstick-client -transport streamable-http -address localhost -port 8081 -action list-tools; then
        echo "✓ Streamable HTTP tools listing successful"
    else
        echo "! Streamable HTTP tools listing failed"
        exit 1
    fi
    
    echo "🔧 Testing tool calling via streamable-http..."
    if ./build/yardstick-client -transport streamable-http -address localhost -port 8081 -action=call-tool -tool=echo -args='{"input":"hellomatey123"}' | grep -q "hellomatey123"; then
        echo "✓ Streamable tool call returned expected output: hellomatey123"
    else
        echo "! Streamable tool call did not return expected output: hellomatey123"
        exit 1
    fi
else
    echo "✗ Streamable HTTP transport container failed to start on port 8081"
    exit 1
fi

# Cleanup streamable-http container
docker stop yardstick-http-test > /dev/null 2>&1
echo "✓ Streamable HTTP container shut down successfully"
###################################################################
############# END - StreamableHTTP TRANSPORT TESTING ##############
###################################################################

###################################################################
################ START - BARRIER MODE FAULT TESTING ################
###################################################################
echo ""
echo "🚧 ========== BARRIER MODE FAULT-INJECTION TESTING =========="
echo "📡 Testing barrier mode on port 8082..."
docker run --rm -d --name yardstick-barrier-test -p 8082:8082 \
    -e BACKEND_MODE=barrier -e BARRIER_N=2 -e BARRIER_TIMEOUT_SECONDS=10 \
    $IMAGE_NAME --transport streamable-http --port 8082 > /dev/null 2>&1
sleep 3

if docker ps | grep -q yardstick-barrier-test; then
    echo "✓ Barrier mode container started successfully on port 8082"

    OUT1=/tmp/yardstick-barrier-1.out
    OUT2=/tmp/yardstick-barrier-2.out
    : > "$OUT1"; : > "$OUT2"

    START_TS=$(date +%s)
    ./build/yardstick-client -transport streamable-http -address localhost -port 8082 \
        -action=call-tool -tool=echo -args='{"input":"barrierone"}' > "$OUT1" 2>&1 &
    PID1=$!
    ./build/yardstick-client -transport streamable-http -address localhost -port 8082 \
        -action=call-tool -tool=echo -args='{"input":"barriertwo"}' > "$OUT2" 2>&1 &
    PID2=$!

    RC1=0; wait $PID1 || RC1=$?
    RC2=0; wait $PID2 || RC2=$?
    END_TS=$(date +%s)
    ELAPSED=$((END_TS - START_TS))

    if [ $RC1 -ne 0 ] || [ $RC2 -ne 0 ]; then
        echo "! Barrier calls failed (rc1=$RC1 rc2=$RC2)"
        exit 1
    fi

    if grep -q "barrierone" "$OUT1" && grep -q "barriertwo" "$OUT2"; then
        echo "✓ Both barrier calls returned their expected echoed values"
    else
        echo "! Barrier call output missing expected echoed value"
        exit 1
    fi

    if [ $ELAPSED -lt 5 ]; then
        echo "✓ Barrier released in ${ELAPSED}s (N=2 arrived, safety timer did not fire)"
    else
        echo "! Barrier took ${ELAPSED}s - looks like the 10s safety timer fired instead of N=2"
        exit 1
    fi
    rm -f "$OUT1" "$OUT2"
else
    echo "✗ Barrier mode container failed to start on port 8082"
    exit 1
fi

docker rm -f yardstick-barrier-test > /dev/null 2>&1
echo "✓ Barrier mode container shut down successfully"
###################################################################
################# END - BARRIER MODE FAULT TESTING ##################
###################################################################

###################################################################
################# START - HANG MODE FAULT TESTING ###################
###################################################################
echo ""
echo "🥶 ========== HANG MODE FAULT-INJECTION TESTING =========="
echo "📡 Testing hang mode on port 8083..."
docker run --rm -d --name yardstick-hang-test -p 8083:8083 \
    -e BACKEND_MODE=hang -e HANG_AFTER_N=1 \
    $IMAGE_NAME --transport streamable-http --port 8083 > /dev/null 2>&1
sleep 3

if docker ps | grep -q yardstick-hang-test; then
    echo "✓ Hang mode container started successfully on port 8083"

    if timeout 8 ./build/yardstick-client -transport streamable-http -address localhost -port 8083 \
        -timeout 3s -action=call-tool -tool=echo -args='{"input":"shouldhang"}' > /dev/null 2>&1; then
        echo "! Hang mode call unexpectedly succeeded"
        exit 1
    else
        echo "✓ Hang mode call failed as expected (client -timeout bound the stuck request)"
    fi
else
    echo "✗ Hang mode container failed to start on port 8083"
    exit 1
fi

# Container never exits on its own in hang mode; force-remove it.
docker rm -f yardstick-hang-test > /dev/null 2>&1
echo "✓ Hang mode container shut down successfully"
###################################################################
################## END - HANG MODE FAULT TESTING #####################
###################################################################

###################################################################
################ START - CRASH MODE FAULT TESTING ####################
###################################################################
echo ""
echo "💥 ========== CRASH MODE FAULT-INJECTION TESTING =========="
echo "📡 Testing crash mode on port 8084..."
# No --rm here: docker wait racing an auto-removed container is a real
# (documented Moby) failure mode, so the container is removed explicitly
# below, after its exit code has been read.
docker run -d --name yardstick-crash-test -p 8084:8084 \
    -e BACKEND_MODE=crash -e CRASH_AFTER_N=1 \
    $IMAGE_NAME --transport streamable-http --port 8084 > /dev/null 2>&1
sleep 3

if docker ps | grep -q yardstick-crash-test; then
    echo "✓ Crash mode container started successfully on port 8084"

    # The connection dies mid-request when the server calls os.Exit(1);
    # a client-side failure here is expected, not asserted on directly.
    ./build/yardstick-client -transport streamable-http -address localhost -port 8084 \
        -action=call-tool -tool=echo -args='{"input":"willcrash"}' > /dev/null 2>&1 || true

    EXIT_CODE=$(timeout 10 docker wait yardstick-crash-test 2>/dev/null || echo timeout)
    if [ "$EXIT_CODE" = "1" ]; then
        echo "✓ Crash mode container exited with code 1 as expected"
    else
        echo "! Crash mode container exited with code '$EXIT_CODE', expected 1"
        exit 1
    fi
else
    echo "✗ Crash mode container failed to start on port 8084"
    exit 1
fi

docker rm -f yardstick-crash-test > /dev/null 2>&1
echo "✓ Crash mode container cleaned up successfully"
###################################################################
################# END - CRASH MODE FAULT TESTING #####################
###################################################################

###################################################################
############### START - STATELESS MODE TESTING ####################
###################################################################
echo ""
echo "🧊 ========== STATELESS MODE TESTING =========="
echo "📡 Testing stateless streamable-http on port 8085..."
docker run --rm -d --name yardstick-stateless-test -p 8085:8085 \
    -e STATELESS=true \
    $IMAGE_NAME --transport streamable-http --port 8085 > /dev/null 2>&1
sleep 3

if docker ps | grep -q yardstick-stateless-test; then
    echo "✓ Stateless mode container started successfully on port 8085"

    if ./build/yardstick-client -transport streamable-http -address localhost -port 8085 \
        -action info | grep -q "Protocol Version: 2026-07-28"; then
        echo "✓ Stateless server negotiated Modern protocol"
    else
        echo "! Stateless server did not report expected protocol version"
        exit 1
    fi

    if ./build/yardstick-client -transport streamable-http -address localhost -port 8085 \
        -action=call-tool -tool=echo -args='{"input":"statelesscheck"}' | grep -q "statelesscheck"; then
        echo "✓ Stateless tool call returned expected output: statelesscheck"
    else
        echo "! Stateless tool call did not return expected output: statelesscheck"
        exit 1
    fi
else
    echo "✗ Stateless mode container failed to start on port 8085"
    exit 1
fi

docker rm -f yardstick-stateless-test > /dev/null 2>&1
echo "✓ Stateless mode container shut down successfully"
###################################################################
################ END - STATELESS MODE TESTING ######################
###################################################################
