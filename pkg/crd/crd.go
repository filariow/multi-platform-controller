package crd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/konflux-ci/multi-platform-controller/pkg/cloud"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	return cloud.InstanceIdentifier(taskRunID), nil
}

func (p *CRDProvider) TerminateInstance(kubeClient client.Client, ctx context.Context, instance cloud.InstanceIdentifier) error {
	// not my business
	return nil
}

// GetInstanceAddress this only returns an error if it is a permanent error and the host will not ever be available
func (p *CRDProvider) GetInstanceAddress(cli client.Client, ctx context.Context, instanceId cloud.InstanceIdentifier) (string, error) {
	err := cloud.ValidateTaskRunID(string(instanceId))
	if err != nil {
		return "", fmt.Errorf("invalid TaskRun ID: %w", err)
	}
	idSplit := strings.Split(string(instanceId), ":")
	ns, tr := idSplit[0], idSplit[1]
	ss := corev1.SecretList{}
	if err := cli.List(ctx, &ss,
		client.InNamespace(ns),
	); err != nil || len(ss.Items) == 0 {
		return "", nil
	}

	pp := corev1.PodList{}
	if err := cli.List(ctx, &pp,
		client.InNamespace(ns),
		client.MatchingLabels{
			"tekton.dev/taskRun": tr,
		}); err != nil || len(pp.Items) == 0 {
		return "", nil
	}

	po := pp.Items[0]
	for _, s := range ss.Items {
		if v, ok := s.GetAnnotations()["mpc.konflux-ci.dev/pod-name"]; ok && v == po.GetName() {
			return string(s.Data["address"]), nil
		}
	}

	return "", nil
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
	panic("crd-user")
}
