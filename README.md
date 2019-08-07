

Demos GC impact on the HTTP server latency
Usage 

    go run .
    httperf --server localhost --port 8081 --uri "/query?key=67"  --num-calls 1000000  --verbose 