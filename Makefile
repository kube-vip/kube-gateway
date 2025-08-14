#vars
VERSION=v1
REPO=thebsdbox
GATEWAYIMAGEFULLNAME=${REPO}/kube-gateway:${VERSION}
DEMOIMAGEFULLNAME=demo/demo:${VERSION}
WATCHERIMAGEFULLNAME=${REPO}/watcher:${VERSION}

.PHONY: help build push all

help:
			@echo "Makefile for the kube-vip gateway"
			@echo ""
			@echo "demo - for building and managing the example program"
			@echo "push"
			@echo "all"

.DEFAULT_GOAL := all

kind:
	@kind create cluster --config ./kind.yaml

demo: build_demo push_demo

build_demo:
	@docker build -t ${DEMOIMAGEFULLNAME} ./demo

push_demo:
	@docker push ${DEMOIMAGEFULLNAME}

kind_demo:
	@kind load docker-image ${DEMOIMAGEFULLNAME}

watcher: build_watcher push_watcher

build_watcher:
	@docker build -t ${WATCHERIMAGEFULLNAME} -f ./Dockerfile_Watcher .

push_watcher:
	@docker push ${WATCHERIMAGEFULLNAME}

kind_watcher:
	@kind load docker-image ${WATCHERIMAGEFULLNAME}

gateway: build_gateway push_gateway

build_gateway:
	@docker build -t ${GATEWAYIMAGEFULLNAME} -f ./Dockerfile_Gateway .

push_gateway:
	@docker push ${GATEWAYIMAGEFULLNAME}

kind_gateway:
	@kind load docker-image ${GATEWAYIMAGEFULLNAME}

kind_clean:
	@kubectl delete -f ./demo/deployment.yaml
	@kubectl delete -f ./watcher/deployment.yaml
