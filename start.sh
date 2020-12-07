sudo docker run --privileged \
  --rm \
  --name fnserver \
  -it \
  -v $PWD/data:/app/data \
  -v $PWD/data/iofs:/iofs \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e "FN_IOFS_DOCKER_PATH=$PWD/data/iofs" \
  -e "FN_IOFS_PATH=/iofs" \
  -p 8080:8080 \
  my_fnserver:v0.1  
