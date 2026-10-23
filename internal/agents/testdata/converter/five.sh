#!/bin/sh
cat > /dev/null
for n in 1 2 3 4 5; do
  echo "{\"v\":1,\"source\":\"acme\",\"session\":\"s$n\",\"kind\":\"started\"}"
done
