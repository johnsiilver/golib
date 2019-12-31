#!/bin/bash

for num in {0..6}
do
	time go test -run=xxx -bench=BenchmarkWithPoolGRPCv3 -args -testnum=$num
done
