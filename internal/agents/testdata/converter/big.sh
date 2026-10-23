#!/bin/sh
cat > /dev/null
head -c 70000 /dev/zero | tr '\0' 'x'
echo
