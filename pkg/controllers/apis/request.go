package apis

import (
	v1 "k8s.io/api/core/v1"
)

// Request struct.
type Request struct {
	ResourceQuota *v1.ResourceQuota
	Verb          string
}
