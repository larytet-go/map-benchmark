

Demos GC impact on the HTTP server latency.

Usage 

    GODEBUG=gctrace=1,schedtrace=500 go run .
    httperf --server localhost --port 8081 --uri "/query?key=magic"  --num-calls 10000000  --verbose 

In a separate terminal

    while [ 1 ];do echo -en "\\033[0;0H";curl http://127.0.0.1:8081/stat;sleep 0.3;done;

My machine 

    $ go version
    go version go1.12.4 linux/amd64
