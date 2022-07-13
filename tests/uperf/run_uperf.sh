#!/bin/bash
set -e

SCALE=8
NTHR=5
ITER=1000000

kubectl delete -f uperf-test || true
kubectl create ns uperf-test
kube="kubectl -n uperf-test"

$kube apply -f net-test.yaml
sleep 5

$kube scale --replicas=$SCALE deployment perf-master-deployment
$kube scale --replicas=$SCALE deployment perf-slave-deployment
$kube wait deployment/perf-master-deploymnet --for condition=available
$kube wait deployment/perf-server-deploymnet --for condition=available

while true; do
    echo SCALE=$SCALE
    echo NTHR=$NTHR
    echo ITER=$ITER
    echo "start test? (y,[n])"
    read input
if [[ "$input" == "y" ]]; then
    break
fi
    $kube get pods -o wide
    sleep 1
done

IFS=$'\n' read -r -d '' -a ips < <( $kube get pods -o wide | grep slave | awk '{print $6}' && printf '\0' )

let i=0
for pod in $($kube get pods -o name | grep master ); do 
    $kube exec $(echo $pod | cut -d "/" -f 2) -- sh -c "export h=${ips[$i]}; export nthr=${NTHR}; export iter=${ITER}; uperf -m /usr/share/uperf/connect.xml -a" | tee uperf_$i &
    let i=$i+1
done

