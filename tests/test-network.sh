#!/bin/bash
# test-network.sh — Verify VM networking works after image creation
#
# Usage: ./tests/test-network.sh <image-name>
# Example: ./tests/test-network.sh ubuntu-slim

set -euo pipefail

IMAGE="${1:?Usage: $0 <image-name>}"
VM_NAME="test-network-${IMAGE}-$$"
SSH_USER="cirun"
SSH_PASS="cirun"
SSH_OPTS="-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=5 -o LogLevel=ERROR"
MAX_WAIT=120  # seconds to wait for IP
PASS=0
FAIL=0

cleanup() {
    echo "--- Cleanup ---"
    meda delete "$VM_NAME" 2>/dev/null || true
}
trap cleanup EXIT

pass() {
    echo "PASS: $1"
    PASS=$((PASS + 1))
}

fail() {
    echo "FAIL: $1"
    FAIL=$((FAIL + 1))
}

echo "=== Network Integration Test ==="
echo "Image: ${IMAGE}"
echo "VM:    ${VM_NAME}"
echo ""

# 1. Check image exists locally
echo "--- Check image exists ---"
if meda images list --json | jq -e ".[] | select(.name == \"${IMAGE}\")" > /dev/null 2>&1; then
    pass "Image '${IMAGE}' exists locally"
else
    echo "FATAL: Image '${IMAGE}' not found locally"
    echo "Available images:"
    meda images list 2>/dev/null || true
    exit 1
fi

# 2. Create VM from image
echo "--- Create VM ---"
if meda run "${IMAGE}:latest" --name "$VM_NAME"; then
    pass "VM created from ${IMAGE}:latest"
else
    echo "FATAL: Failed to create VM"
    exit 1
fi

# 3. Wait for VM to get an IP
echo "--- Wait for IP (max ${MAX_WAIT}s) ---"
VM_IP=""
elapsed=0
while [ $elapsed -lt $MAX_WAIT ]; do
    VM_IP=$(meda ip "$VM_NAME" 2>/dev/null || true)
    if [ -n "$VM_IP" ] && [ "$VM_IP" != "null" ]; then
        break
    fi
    sleep 5
    elapsed=$((elapsed + 5))
    echo "  waiting... (${elapsed}s)"
done

if [ -n "$VM_IP" ] && [ "$VM_IP" != "null" ]; then
    pass "VM got IP: ${VM_IP}"
else
    fail "VM did not get an IP within ${MAX_WAIT}s"
    echo "FATAL: Cannot continue without IP"
    exit 1
fi

# 4. Check ARP entry is not (incomplete)
echo "--- Check ARP ---"
arp_entry=$(arp -n "$VM_IP" 2>/dev/null || true)
if echo "$arp_entry" | grep -q "(incomplete)"; then
    fail "ARP entry for ${VM_IP} is (incomplete) — guest NIC not configured"
else
    pass "ARP entry for ${VM_IP} is valid"
fi

# 5. Wait for SSH and verify connectivity
echo "--- Check SSH connectivity ---"
ssh_ok=false
ssh_elapsed=0
ssh_max=60
while [ $ssh_elapsed -lt $ssh_max ]; do
    if sshpass -p "$SSH_PASS" ssh $SSH_OPTS "${SSH_USER}@${VM_IP}" "echo ok" > /dev/null 2>&1; then
        ssh_ok=true
        break
    fi
    sleep 5
    ssh_elapsed=$((ssh_elapsed + 5))
    echo "  waiting for SSH... (${ssh_elapsed}s)"
done

if $ssh_ok; then
    pass "SSH connectivity to ${VM_IP}"
else
    fail "SSH connectivity to ${VM_IP} (timed out after ${ssh_max}s)"
    echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
    exit 1
fi

# 6. Check cloud-init status inside the VM
echo "--- Check cloud-init status ---"
ci_status=$(sshpass -p "$SSH_PASS" ssh $SSH_OPTS "${SSH_USER}@${VM_IP}" \
    "cloud-init status 2>/dev/null" || true)
if echo "$ci_status" | grep -q "done"; then
    pass "cloud-init status is done"
else
    fail "cloud-init status: ${ci_status}"
fi

# 7. Verify netplan config has the correct IP
echo "--- Check netplan config ---"
netplan_ip=$(sshpass -p "$SSH_PASS" ssh $SSH_OPTS "${SSH_USER}@${VM_IP}" \
    "grep -oP '\\d+\\.\\d+\\.\\d+\\.\\d+' /etc/netplan/50-cloud-init.yaml 2>/dev/null | head -1" || true)
if [ "$netplan_ip" = "$VM_IP" ]; then
    pass "Netplan config has correct IP (${netplan_ip})"
else
    fail "Netplan IP mismatch: expected ${VM_IP}, got '${netplan_ip}'"
    echo "  Netplan contents:"
    sshpass -p "$SSH_PASS" ssh $SSH_OPTS "${SSH_USER}@${VM_IP}" \
        "cat /etc/netplan/50-cloud-init.yaml 2>/dev/null" || true
fi

# Summary
echo ""
echo "=== Results: ${PASS} passed, ${FAIL} failed ==="
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
