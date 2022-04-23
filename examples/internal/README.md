# Overrview

janus-gateway/examples/internal 是使用janus-gateway的一个完整示例，包含gateway-server 和 gRPC-server. 还有Java实现gRPC Server的例子 [https://github.com/binchencoder/spring-boot-grpc/tree/master/spring-boot-grpc-examples]

# Build the example

build gateway server
```
bazel build examples/internal/cmd/example-gateway-server/... 
```

build gRPC server
```
bazel build examples/internal/cmd/example-grpc-server/...
```

# Run the example

start gateway server
```shell
janus-gateway/bazel-bin/examples/internal/cmd/example-gateway-server/example-gateway-server_/example-gateway-server -skylb-endpoints="127.0.0.1:1900" -debug-svc-endpoint=custom-janus-gateway-test=localhost:9090
```

start custom-gateway server
```shell
janus-gateway/bazel-bin/cmd/custom-gateway/custom-janus-gateway_/custom-janus-gateway -skylb-endpoints="127.0.0.1:1900" -debug-service=custom-janus-gateway-test -debug-svc-endpoint=custom-janus-gateway-test=localhost:9090
```

start examples gRPC server for test //examples/internal/cmd/example-grpc-server
```shell
janus-gateway/bazel-bin/examples/internal/cmd/example-grpc-server/example-grpc-server_/example-grpc-server -skylb-endpoints="127.0.0.1:1900,127.0.0.1:1901"
```

start gRPC server for test //cmd/gateway
```shell
janus-gateway/bazel-bin/examples/grpc-server/grpc-server_/grpc-server -skylb-endpoints="127.0.0.1:1900"
```

start //cmd/gateway
```shell
janus-gateway/bazel-bin/cmd/gateway/gateway_/gateway -skylb-endpoints="127.0.0.1:1900" -v=2 -log_dir=.
```

# Usage

You can use curl or a browser to test:
```
# List all apis
$ curl http://localhost:8080/swagger/echo_service.swagger.json

# Visit the apis
$ curl -XPOST  -H "x-source: web" http://localhost:8080/v1/example/echo/foo
{"id":"foo"}

$ curl -H "x-source: web"  http://localhost:8080/v1/example/echo/foo/123
{"id":"foo","num":"123"}

$ curl -XDELETE -H "x-source: web"  http://localhost:8080/v1/example/echo_delete

$ curl -XPOST -H "Content-Type:application/json" -H "x-source:web" -d '{"id": "11", "num": 1}' http://localhost:8080/v1/example/echo_body
```

> NOTE: 请注意当前用户是否是管理员