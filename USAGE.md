# Fn Document

## Fn Introduction

The Fn project is an open-source container-native serverless platform, which supports various languages even custom binaries.

## Pre-requisites

Docker 17.10.0-ce or later installed and running. Docker is also required on the machine installed with fn cli tool.

Download the latest release of fn cli tool from [https://github.com/fnproject/cli/releases](https://github.com/fnproject/cli/releases) and move the executable to your $PATH

## Get Started

We will build a single-machine serverless platform with the Fn.

### Single-machine deployment (using pre-built binaries)

```bash
sudo fn start
```

"fn start" will use a pre-built image on [https://hub.docker.com/r/fnproject/fnserver](https://hub.docker.com/r/fnproject/fnserver)

### Single-machine deployment (building customized version)

```bash
git clone https://ipads.se.sjtu.edu.cn:1312/distributed-rdma-serverless/fn.git

cd fn && git checkout v0.3.749

./build.sh my_fnserver:v0.1 # this will build an image tagged with my_fnserver:v0.1

docker run --privileged \ # we need to add --privileged to enable the fn in the container to launch a container
  --rm \
  --name fnserver \
  -it \ # here we start it iteratively for developing
  -v $PWD/data:/app/data \
  -v $PWD/data/iofs:/iofs \ # (Optional) We can map some metadata directories to the host to maintain the state after one container exits
  -v /var/run/docker.sock:/var/run/docker.sock \ # (Necessary) We need to map the docker.sock so that the fn can use the outside docker engine
  -e "FN_IOFS_DOCKER_PATH=$PWD/data/iofs" \
  -e "FN_IOFS_PATH=/iofs" \
  -p 8080:8080 \
  my_fnserver:v0.1

# Optional, if you want to push it to a docker registry
docker tag my_fnserver:v0.1 val01:5000/my_fnserver:v0.1
```

Both of them will start a single-machine standalone server. Use control-c to quit.

### Client

We will use the fn cli tool to create, deploy and invoke a function.

```bash
fn init --runtime go --trigger http gofn # init a new go function with http trigger
cd gofn # enter the directory
fn create app goapp # invoke the fn server to create a new app
fn --verbose deploy --app goapp --local # this will create an image for this function and deploy it under the newly-created "goapp" app
fn invoke goapp gofn # invoke the function
```

## Deployment in distributed environment

### Pre-requisites

A local docker registry need to be deployed, or we can use [hub.docker.com](http://hub.docker.com) to upload/download images. A local docker registry is available on val01:5000.

### Overall architecture

![architecture](./assets/architecture.png)

We need to deploy a **MySQL** instance to store the metadata of the functions. The database usage is kept to a minimum, and there is no write/read on a function invocation's critical path when there is no cache (software cache managed by fn) miss.

```bash
# An instance of MySQL is active on val08:33306
export FN_MYSQL_PORT=33306
docker run --name fn-mysql \
	-e MYSQL_DATABASE=funcs \
  -e MYSQL_ROOT_PASSWORD=root \ # root user with password "root"
	-d \
	-p ${FN_MYSQL_PORT}:3306 \ # Map the MySQL port to the port on the host
	mysql:5.7.22
```

A memcached (or other types of key-value stores) is needed to serve as a S3-like infrastructure to hold the intermediate data of functions.

```bash
# An instance of memcached is active on val08:31211
docker run -d --network fnproject -p 31211:11211 \
  --name fn-memcached \
  memcached:1.6.9-alpine -I 1024M -m 2048M # we need these arguments to allow larger key-value pairs
```

A typical cluster contains **1 API node (metadata management)**, **1 LB/Scheduler node (load balance, scheduling)** and **several (currently 2) Runner node (function runner)**.

All of these three components are scalable.

According to the configuration described above, we expose some variables first. (Configurations on val cluster, for example)

```bash
export FN_MYSQL_VAL=val08
export FN_MYSQL_PORT=33306
export FN_API_VAL=val09
export FN_LB_VAL=val13
export FN_RUNNER_1=val10
export FN_RUNNER_2=val11
export VAL_DOCKER_REGISTRY=val01:5000
```

Deploy an API node on val09, act as root user and execute these lines

```bash
docker pull ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1 && \
docker run --name fn-api --rm -it \
        -e FN_NODE_TYPE=api \
        -e FN_PUBLIC_LB_URL="http://${FN_LB_VAL}:8082" \
        -e FN_DB_URL="mysql://root:root@tcp(${FN_MYSQL_VAL}:${FN_MYSQL_PORT})/funcs" \
        -v /var/run/docker.sock:/var/run/docker.sock \
        --privileged \
        -p 8080:8080 \
        ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1
```

Deploy a load balancer/scheduler on val13

```bash
docker pull ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1 && \
docker run --name fn-runner-lb --rm -it \
        --privileged \
        --ulimit nofile=32768:32768 \ # increase the nofile for more concurrency
        -v /var/run/docker.sock:/var/run/docker.sock \
        -e FN_NODE_TYPE=lb \
        -e FN_RUNNER_API_URL="http://${FN_API_VAL}:8080" \
        -e FN_RUNNER_ADDRESSES="${FN_RUNNER_1}:9190,${FN_RUNNER_2}:9190" \ # specify the runners here
        -p 8082:8080 \
        ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1
```

Deploy a runner node on val10

```bash
docker pull ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1 && \
docker run --name fn-runner-1 --rm -it \
        --privileged \
        --ulimit nofile=32768:32768 \
        -e FN_NODE_TYPE="pure-runner" \
        -e FN_IOFS_DOCKER_PATH=${PWD}/iofs \
        -e FN_IOFS_PATH=/iofs \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v ${PWD}/iofs:/iofs \
        -p 9190:9190 \
        ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1
```

Deploy a runner node on val11

```bash
docker pull ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1 && \
docker run --name fn-runner-2 --rm -it \
        --privileged \
        --ulimit nofile=32768:32768 \
        -e FN_NODE_TYPE="pure-runner" \
        -e FN_IOFS_DOCKER_PATH=${PWD}/iofs \
        -e FN_IOFS_PATH=/iofs \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v ${PWD}/iofs:/iofs \
        -p 9190:9190 \
        ${VAL_DOCKER_REGISTRY}/my_fnserver:v0.1
```

Configure the fn cli on your own machine, which is not necessary to be in the fn cluster.

The api server runs on val09:8080, docker registry runs on val01:5000.

```bash
fn create context val-context --api-url http://val09:8080 --registry val01:5000
```

### Deployment and Invocation of a function chain

#### Simple Reverse String

First, we should deploy a runnable string-reverse function.

```bash
git clone https://ipads.se.sjtu.edu.cn:1312/distributed-rdma-serverless/fn-playground.git
cd fn-playground/revapp/reverse-string
fn create app revapp # create a "revapp" app, it is fine if fn complains that "App already exists"
# "fn deploy" command will build the docker image, push the image to the registry specified above (--registry) and write the function metadata through (--api-url).
fn --verbose deploy --app revapp
# "fn ls triggers <app>" will show all the http trigger endpoints of an app
fn ls triggers revapp
curl -X POST -d 'hello' http://val13:8082/t/revapp/reverse-string # use curl to invoke the function, it will returns 'olleh'
```

We will invoke the function chain. `fn-playground/rev_pipeline.json` defines a simple function chain which reverses the input string twice and returns the final result.

```json
{
	"Comment": "",
	"StartAt": "start",
	"States": {
		"start": {
			"Type": "Task",
			"AppName": "revapp",
			"FuncName": "/reverse-string",
			"Next": "end",
			"Comment": "",
			"End": false
		},
		"end": {
			"Type": "Task",
			"AppName": "revapp",
			"FuncName": "/reverse-string",
			"Next": "",
			"Comment": "",
			"End": true
		}
	}
}

Invoke the pipeline with curl. Make sure you are in the same directory with rev_pipeline.json.

```bash
curl -X POST -d @rev_pipeline.json --header 'Input-String: Hello' http://val13:8082/schedule # access the /schedule endpoint of the fn scheduler, and it will return Hello
```
