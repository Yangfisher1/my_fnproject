if [ "$1" = "" ]
then
  file="benchmark.json"
else
  file="$1"
fi
#echo "curl -X POST --header 'Input-String: Hello' -d @$file http://val12:8082/benchmark"
curl -X POST --header 'Input-String: Hello' -d @$file http://val12:8082/benchmark
