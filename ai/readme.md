# AI experiments

Had to happen eventually :|

## Create a quick 2 node Kind cluster
```
kind create cluster --config ./kind/config.yaml 
```

## Apply the workload

This will deplot ollama with a 7GB local llm.

```
kind apply -f ./everything.yaml
```

## Develop with Skaffold

In the `/aiClient` folder start skaffold:

```
cd aiClient
skaffold dev
```



## Debugging

### get ollama service address
ollama=`kubectl get svc -n ollama ollama -o json | jq .spec.clusterIP`

### Test connectivity
docker exec -it services-worker curl http://$ollama:11434
docker inspect services-worker | jq -r '.[] | .NetworkSettings.Networks.kind.IPAddress'
