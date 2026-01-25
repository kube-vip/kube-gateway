# kube-gateway

![](https://github.com/kube-vip/kube-vip/raw/main/kube-vip.png)

A transparent gateway for Kubernetes pods

**Note** this is more of a proof of concept at this stage, so feel free to take for a test drive and give feedback. Additionally it uses ephemeral containers to attach the proxy to your workloads and due to a bug in previous releases of Kubernetes only works from version v1.33 onwards. 

## Architecture

A workload watcher/informer watches for all pods (especially their `update`) call, as when a pod is `created` it won't have an IP address. The `update` occurs once an IP address has been applied, we can the use this IP address to mint a certificate for the pod and create a secret for the pod. Finally an ephemeral container is created and added to the pod, making use of that secret to allow for encrypted communication.

### Why ephemeral containers?

Well, they're pretty cool and they are a good way to attach things to an already running pod! With kube-gateway we create a ephemeral container that also loads a secret (which was broken prior to v1.33) that has the created certificates, eBPF redirects traffic to the gateway and that then encrypts it and sends it on it's merry way.

### Code
- `demo` contains a simple demonstration of two pods speaking to one another over TCP (no encryption)
- `ebpf` contains the eBPF code for redirecting connections to the proxy
- `gateway` contains the code for the userland portion speaking with the eBPF and TLS connections
- `pkg` contains shared code
- `watcher` contains the pod watcher code


## Encryption 🔐

### Create a cluster (MUST be v1.33+)

`make kind`

### Create the CA (Certificate Authority)

The following commands will generate the certificate authority

```openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt
kubectl create secret generic watcher --from-file=ca-key=ca.key --from-file=ca-cert=ca.crt
```

or you can use the `watcher` to create the `secret` that has the CA certs:
```
cd watcher
go run main.go -ca
go run main.go -loadca
```

### Create the watcher!

The watcher will watch for pods that have an IP address **and** the correct annotation (kube-gateway.io=true).

`kubectl apply -f ./watcher/deployment.yaml`

### Apply a workload (demo)

Start the demo workload (unencrypted)!

`kubectl apply -f ./demo/deployment.yaml`

You can use tcpdump/wireshark to watch the traffic unencrypted flying back and forth.


#### To use kTLS (in-kernel TLS)

This has to be done **first** for kTLS offload to be enabled.

`kubectl annotate pod <pod name> kube-gateway.io/ktls="true"`

#### Enable Encryption between pods

This will apply the gateway to pod-01:
`kubectl annotate pod pod-01 kube-gateway.io/encrypt="true"`

This will then apply the other gateway to pod-02:
`kubectl annotate pod pod-02 kube-gateway.io/encrypt="true"`

At which point all traffic will be encrypted end-to-end 🤩

## AI 🤖

### Create a cluster (MUST be v1.33+)

`make kind`

Apply the gateway to the pod with the following:
`kubectl annotate pod pod-01 kube-gateway.io/ai="true"`

### Deploy an example

We're going to deploy ollama with the `llama3.2` model.

```
kubectl apply -f ./AI/everything.yaml
```

Then we're going to deploy our own AI client:

```
kubectl apply -f ./AI/aiClient/pod.yaml
```

We can deploy a second too (optional), this one uses python but is another good example of manipulating multiple workloads:

```
kubectl apply -f ./AI/aiClient-py/pod.yaml
```

### Override example (model type)
Make sure that the model has been pulled into ollama first :-) 

```
curl -X POST http://172.18.0.4:30007/api/pull -H 'Content-Type: application/json' -d '{"name": "gemma2:2b"}'
```

`kubectl annotate pod aipod kube-gateway.io/ai-model="gemma2:2b"`

kubectl create configmap <pod>-kube-gateway --from-file=config=./path.json

## Debugging

You can see the logs of the gateway with the following: 
`kubectl logs pod-01 -c kube-gateway`


# Overview

## Original Architecture
```
┌───────────┐          ┌───────────┐
│ Pod01     │          │ Pod02     │
│ 10.0.0.1  ┼────┬─────► 10.0.0.2  │
└───────────┘    │     └───────────┘
                 │                  
                 │                  
                 │                  
            CNI Magic 🧙🏻‍♂️
```

## Gateway attached

```
┌─────────────────────────────────┐                     ┌─────────────────────────────────┐
│Pod-01                           │                     │                           Pod-02│
│10.0.0.1 x─x─x─x─► 10.0.2.2:80   │                     │     ┌────────────────►  10.0.2.2│
│   │  eBPF captures the socket   │                     │     │   :80                     │
│   │  Finds original destination │                     │     │                           │
│   │  Changes destination to lo  │                     │     │                           │
│   │                             │                     │     │                           │
│   ▼  Our TLS listener sends     │                     │     │                           │
│127.0.0.1:18000                  │                     │0.0.0.0:18001                    │
│         │                       │                     │     ▲                           │
└─────────┼───────────────────────┘                     └─────┼───────────────────────────┘
          │                                                   │                            
          └────────────────────────🔐─────────────────────────┘                            
            Uses original destination with a modified port                                 
```

### TLS in action

#### Without the sidecar

The original port of `9000` is still being send cleartext traffic.

```
    10.0.0.227.35928 > 10.0.1.54.9000: Flags [P.], cksum 0x1650 (incorrect -> 0xd116), seq 153:170, ack 1, win 507, options [nop,nop,TS val 1710156213 ecr 1761501942], length 17
	0x0000:  4500 0045 8b67 4000 4006 9933 0a00 00e3  E..E.g@.@..3....
	0x0010:  0a00 0136 8c58 2328 ed78 5fcb 31aa 5b9b  ...6.X#(.x_.1.[.
	0x0020:  8018 01fb 1650 0000 0101 080a 65ee e9b5  .....P......e...
	0x0030:  68fe 62f6 4865 6c6c 6f20 6672 6f6d 2070  h.b.Hello.from.p
	0x0040:  6f64 2d30 31                             od-01
```

#### With the sidecar

We can see that the destination port has been changed to the TLS port `18443`. 
```
    10.0.0.196.51740 > 10.0.1.132.18443: Flags [P.], cksum 0x1695 (incorrect -> 0xef2a), seq 1740:1779, ack 1827, win 502, options [nop,nop,TS val 3093655397 ecr 4140148653], length 39
	0x0000:  4500 005b 7b63 4000 4006 a8f2 0a00 00c4  E..[{c@.@.......
	0x0010:  0a00 0184 ca1c 480b 8a63 4d53 f134 4176  ......H..cMS.4Av
	0x0020:  8018 01f6 1695 0000 0101 080a b865 6f65  .............eoe
	0x0030:  f6c5 a7ad 1703 0300 2244 536d cf88 3385  ........"DSm..3.
	0x0040:  263d d632 3795 b6b7 76c4 177d efee 9331  &=.27...v..}...1
	0x0050:  2dcb 7c3e 5c16 7af6 9164 eb              -.|>\.z..d.
```