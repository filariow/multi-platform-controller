package crd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/konflux-ci/multi-platform-controller/pkg/cloud"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type CRDProvider struct {
	Platform        string
	SystemNamespace string
}

var _ cloud.CloudProvider = &CRDProvider{}

func CreateCRDProvider(platform string, config map[string]string, systemNamespace string) cloud.CloudProvider {
	return &CRDProvider{
		Platform:        platform,
		SystemNamespace: systemNamespace,
	}
}

// For this Provider we do not expect to launch instances
// we just wait for the CRD to be there
func (p *CRDProvider) LaunchInstance(kubeClient client.Client, ctx context.Context, taskRunID string, instanceTag string, additionalInstanceTags map[string]string) (cloud.InstanceIdentifier, error) {
	err := cloud.ValidateTaskRunID(taskRunID)
	if err != nil {
		return "", fmt.Errorf("invalid TaskRun ID: %w", err)
	}

	id := strings.ReplaceAll(taskRunID, ":", "_")
	return cloud.InstanceIdentifier(id), nil
}

func (p *CRDProvider) TerminateInstance(kubeClient client.Client, ctx context.Context, instance cloud.InstanceIdentifier) error {
	// not my business
	return nil
}

// GetInstanceAddress this only returns an error if it is a permanent error and the host will not ever be available
func (p *CRDProvider) GetInstanceAddress(cli client.Client, ctx context.Context, instanceId cloud.InstanceIdentifier) (string, error) {
	trid := strings.ReplaceAll(string(instanceId), "_", ":")
	err := cloud.ValidateTaskRunID(trid)
	if err != nil {
		return "", fmt.Errorf("invalid TaskRun ID: %w", err)
	}

	idSplit := strings.Split(string(instanceId), "_")
	ns, tr := idSplit[0], idSplit[1]
	l := log.FromContext(ctx, "instance-id", instanceId)
	l.Info("get instance address")

	s := corev1.Secret{}
	k := types.NamespacedName{Namespace: ns, Name: tr + "-pod"}
	if err := cli.Get(ctx, k, &s); err != nil {
		l.Error(err, "error retrieving secret", "key", k)
		return "", nil
	}

	return string(s.Data["address"]), nil
}

func (p *CRDProvider) CountInstances(kubeClient client.Client, ctx context.Context, instanceTag string) (int, error) {
	// we don't care about counting them here
	return 0, nil
}

func (p *CRDProvider) ListInstances(kubeClient client.Client, ctx context.Context, instanceTag string) ([]cloud.CloudVMInstance, error) {
	panic("dynamicpool is not supported")
}

func (p *CRDProvider) GetState(kubeClient client.Client, ctx context.Context, instanceId cloud.InstanceIdentifier) (cloud.VMState, error) {
	// TODO: implement max retry mechanism
	return cloud.OKState, errors.New("retry to get the value quickly")
}

func (p *CRDProvider) CleanUpVms(ctx context.Context, kubeClient client.Client, existingTaskRuns map[string][]string) error {
	panic("not supported")
}

func (p *CRDProvider) SshUser() string {
	return "crd-user"
}
