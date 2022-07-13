#!/bin/bash
# Script to display basic upcall information from all nodes of the Openshift cluster.

set -e

nodes=${@-$(oc get nodes -o name | grep worker | cut -d "/" -f 2)}

printf "%-20s %-20s %-20s\n" "NODE" "DROPS" "FLOWS"
for node in $nodes ; do
        pod=$(oc get pods -o name -n openshift-ovn-kubernetes --field-selector spec.nodeName=$node | grep ovnkube-node | cut -d "/" -f 2)                                                                                                      
        run="oc exec -it -n openshift-ovn-kubernetes $pod -c ovn-controller -- sh -c "
        flows=$($run "ovs-appctl upcall/show  | grep flows | cut -d ':' -f 2 " | tr -d '\r')
        drops=$($run "ovs-appctl dpctl/show | grep -oP -e 'lost:\K\d+'" | tr -d '\r')
        printf "%-20s %-20s %-20s\n" "$node" "$drops" "$flows"
done
