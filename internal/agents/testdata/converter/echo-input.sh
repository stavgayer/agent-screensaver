#!/bin/sh
title=$(cat)
echo "{\"v\":1,\"source\":\"acme\",\"session\":\"s1\",\"kind\":\"started\",\"title\":\"$title\"}"
