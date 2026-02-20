#!/bin/bash

echo "=========================================="
echo "BitTorrent Client Test Script"
echo "=========================================="
echo ""

# Build the client
echo "Step 1: Building the client..."
go build -o torrent-client ./cmd/main.go
if [ $? -ne 0 ]; then
    echo "❌ Build failed!"
    exit 1
fi
echo "✅ Build successful!"
echo ""

# Test 1: Help command
echo "Step 2: Testing help command..."
./torrent-client --help
echo ""

# Test 2: Error handling (no arguments)
echo "Step 3: Testing error handling..."
./torrent-client 2>&1 | grep -q "Usage"
if [ $? -eq 0 ]; then
    echo "✅ Error handling works!"
else
    echo "❌ Error handling failed!"
fi
echo ""

echo "=========================================="
echo "Ready to test with a real torrent!"
echo "=========================================="
echo ""
echo "To test with a real download, try one of these:"
echo ""
echo "1. Small test (Debian netinst ~600MB):"
echo "   wget https://cdimage.debian.org/debian-cd/current/amd64/bt-cd/debian-12.5.0-amd64-netinst.iso.torrent"
echo "   ./torrent-client debian-12.5.0-amd64-netinst.iso.torrent"
echo ""
echo "2. Ubuntu ISO (~4.5GB):"
echo "   wget https://releases.ubuntu.com/22.04/ubuntu-22.04.3-desktop-amd64.iso.torrent"
echo "   ./torrent-client ubuntu-22.04.3-desktop-amd64.iso.torrent"
echo ""
echo "3. With custom settings:"
echo "   ./torrent-client -workers 30 -verbose your-file.torrent"
echo ""
