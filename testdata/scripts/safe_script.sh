#!/bin/bash
# A safe shell script — just basic operations.

# Variable assignments
NAME="world"
COUNT=5

# Simple echo
echo "Hello, $NAME!"

# A loop using seq directly
seq 1 $COUNT | while read i; do
    echo "Iteration $i"
done

# File listing (read-only)
ls -la /tmp

# Conditional
if [ -f /etc/hostname ]; then
    echo "Hostname file exists"
fi

# String manipulation
GREETING="Hello World"
echo "${GREETING,,}"
echo "${#GREETING}"

# Simple arithmetic using let
let "RESULT = 3 + 4"
echo "Result: $RESULT"

# Read-only file operations
cat /etc/hostname 2>/dev/null || echo "Could not read hostname"
wc -l /etc/passwd
