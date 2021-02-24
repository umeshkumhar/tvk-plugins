package test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/trilioData/k8s-triliovault/internal"
	"github.com/trilioData/k8s-triliovault/internal/kube"
	"github.com/trilioData/k8s-triliovault/internal/utils"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes/scheme"
)

type SuiteCommon struct {
	KubeAccessor *kube.Accessor
	Namespace    string
	TestID       string
}

func (s *SuiteCommon) RunTestWithNamespace(m *testing.M) int {
	s.KubeAccessor.CatchInterrupt(s.Namespace)

	defer func() {
		if err := s.KubeAccessor.DeleteNamespace(s.Namespace); err != nil {
			log.Errorf("Error Deleting Namespace, %s", err)
			panic(err)
		}
	}()
	if err := s.KubeAccessor.CreateNamespace(s.Namespace); err != nil {
		log.Error(err)
		return 1
	}

	return m.Run()
}

func NewTestSuite() (*SuiteCommon, error) {
	tID := internal.GenerateRandomString(utils.TestIDLength, false)
	ns := strings.Join([]string{utils.DataMoverNamespace, tID}, "-")
	if ns == "" {
		log.Error("Namespace not set.")
		return nil, fmt.Errorf("namespace not set")
	}

	acc, err := kube.NewEnv(scheme.Scheme)
	if err != nil {
		log.Errorf("Failed to create a new environment: %s", err)
		return nil, err
	}
	return &SuiteCommon{
		KubeAccessor: acc,
		Namespace:    ns,
		TestID:       tID,
	}, nil
}
