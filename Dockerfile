# build stage
FROM registry.cn-shanghai.aliyuncs.com/wangtianxia/fn_build_env:latest AS build-env
ENV D=/go/src/github.com/fnproject/fn
ADD . $D
RUN cd $D/cmd/fnserver && go build -o fn-alpine && cp fn-alpine /tmp/

# final stage: the local fnproject/dind:latest will be either built afresh or
# whatever is the latest from master, depending on whether we're releasing
# a newer cut.
FROM fnproject/dind:latest
WORKDIR /app
COPY --from=build-env /tmp/fn-alpine /app/fnserver
CMD ["./fnserver"]
EXPOSE 8080
