if [ "$1" = "" ]
then
  file="benchmark.result"
else
  file="$1"
fi

i=0
while (( $i < 1000 ))
do
  ./benchmark.sh benchmark.json >> ${file}
  echo "" >> ${file}
  let "i++"
done
