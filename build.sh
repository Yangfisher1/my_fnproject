if test -z $1
then
  echo "docker build --build-arg HTTPS_PROXY --build-arg HTTP_PROXY -t my_fnserver:v0.1 ."
  docker build --build-arg HTTPS_PROXY --build-arg HTTP_PROXY -t my_fnserver:v0.1 .
else
  echo "docker build --build-arg HTTPS_PROXY --build-arg HTTP_PROXY -t "$1" ."
  docker build --build-arg HTTPS_PROXY --build-arg HTTP_PROXY -t $1 .
fi
