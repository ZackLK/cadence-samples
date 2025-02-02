package main

import (
	"context"
	"flag"
	"github.com/google/uuid"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.uber.org/cadence/compatibility"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"
	"go.uber.org/zap"
	"time"

	"go.uber.org/cadence/client"
	"go.uber.org/cadence/worker"
)

const (
	tasklist     = "cross-domain-tl"
	domain0      = "domain0"
	domain1      = "domain1"
	portCluster0 = "127.0.0.1:7833"
	portCluster1 = "127.0.0.1:8833"
)

func main() {
	var mode string
	flag.StringVar(&mode, "m", "trigger", "Mode is worker, trigger.")
	flag.Parse()
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}

	switch mode {
	case "worker0":
		setupWorker(domain0, tasklist, portCluster0, []interface{}{wf0}, []interface{}{activity0})
		setupWorker(domain1, tasklist, portCluster1, []interface{}{wf1}, []interface{}{activity1})
		logger.Info("workers running for cluster 0....")
		select {}
	case "worker1":
		setupWorker(domain1, tasklist, portCluster1, []interface{}{wf1}, []interface{}{activity1})
		logger.Info("workers running for cluster 1....")
		select {}
	case "cross-cluster":
		client1 := setupClient(domain0, portCluster0)
		id := uuid.New().String()
		res, err := client1.StartWorkflow(context.Background(), client.StartWorkflowOptions{
			ID:                           id,
			TaskList:                     tasklist,
			ExecutionStartToCloseTimeout: 30 * time.Second,
		}, wf0, "args")
		if err != nil {
			logger.Error("error starting workflow", zap.Error(err))
		}
		logger.Info("started workflow for domain0", zap.String("wf-id", id), zap.Any("start-wf", res))
	}
}

func setupClient(domain string, hostport string) client.Client {
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: "client",
		Outbounds: yarpc.Outbounds{
			"cadence-frontend": {Unary: grpc.NewTransport().NewSingleOutbound(hostport)},
		},
	})

	err := dispatcher.Start()
	if err != nil {
		panic(err)
	}

	clientConfig := dispatcher.ClientConfig("cadence-frontend")

	svc := compatibility.NewThrift2ProtoAdapter(
		apiv1.NewDomainAPIYARPCClient(clientConfig),
		apiv1.NewWorkflowAPIYARPCClient(clientConfig),
		apiv1.NewWorkerAPIYARPCClient(clientConfig),
		apiv1.NewVisibilityAPIYARPCClient(clientConfig),
	)

	return client.NewClient(
		svc,
		domain,
		&client.Options{
			FeatureFlags: client.FeatureFlags{
				WorkflowExecutionAlreadyCompletedErrorEnabled: true,
			},
		})
}
func setupWorker(domain string, tl string, hostport string, wfs []interface{}, activities []interface{}) {
	dispatcher := yarpc.NewDispatcher(yarpc.Config{
		Name: "client",
		Outbounds: yarpc.Outbounds{
			"cadence-frontend": {Unary: grpc.NewTransport().NewSingleOutbound(hostport)},
		},
	})

	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	err = dispatcher.Start()
	if err != nil {
		panic(err)
	}
	workerOptions := worker.Options{
		Logger: logger,
		FeatureFlags: client.FeatureFlags{
			WorkflowExecutionAlreadyCompletedErrorEnabled: true,
		},
	}

	clientConfig := dispatcher.ClientConfig("cadence-frontend")

	svc := compatibility.NewThrift2ProtoAdapter(
		apiv1.NewDomainAPIYARPCClient(clientConfig),
		apiv1.NewWorkflowAPIYARPCClient(clientConfig),
		apiv1.NewWorkerAPIYARPCClient(clientConfig),
		apiv1.NewVisibilityAPIYARPCClient(clientConfig),
	)

	worker := worker.New(svc, domain, tl, workerOptions)
	for i := range wfs {
		worker.RegisterWorkflow(wfs[i])
	}
	for i := range activities {
		worker.RegisterActivity(activities[i])
	}
	err = worker.Start()
	if err != nil {
		panic(err)
	}
}
