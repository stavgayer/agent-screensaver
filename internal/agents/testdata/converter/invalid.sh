#!/bin/sh
cat > /dev/null
echo '{"v":1,"source":"acme","session":"s1","kind":"paused"}'
echo 'not json'
echo '{"source":"acme","session":"s2","kind":"finished"}'
