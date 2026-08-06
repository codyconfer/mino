package argocd

import "time"

type applicationList struct {
	Metadata listMetadata  `json:"metadata"`
	Items    []application `json:"items"`
}

type listMetadata struct {
	ResourceVersion string `json:"resourceVersion"`
}

type application struct {
	Metadata objectMetadata    `json:"metadata"`
	Spec     applicationSpec   `json:"spec"`
	Status   applicationStatus `json:"status"`
}

type objectMetadata struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type applicationSpec struct {
	Project     string           `json:"project"`
	Destination destination      `json:"destination"`
	Source      *applicationSrc  `json:"source"`
	Sources     []applicationSrc `json:"sources"`
}

type destination struct {
	Server    string `json:"server"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type applicationSrc struct {
	RepoURL        string `json:"repoURL"`
	Path           string `json:"path"`
	TargetRevision string `json:"targetRevision"`
	Chart          string `json:"chart"`
}

type applicationStatus struct {
	Sync           syncStatus        `json:"sync"`
	Health         healthStatus      `json:"health"`
	OperationState *operationState   `json:"operationState"`
	Resources      []resourceStatus  `json:"resources"`
	History        []revisionHistory `json:"history"`
	Conditions     []applicationCond `json:"conditions"`
	ReconciledAt   *time.Time        `json:"reconciledAt"`
}

type syncStatus struct {
	Status   string `json:"status"`
	Revision string `json:"revision"`
}

type healthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type operationState struct {
	Phase      string      `json:"phase"`
	Message    string      `json:"message"`
	StartedAt  *time.Time  `json:"startedAt"`
	FinishedAt *time.Time  `json:"finishedAt"`
	RetryCount int         `json:"retryCount"`
	Operation  operation   `json:"operation"`
	SyncResult *syncResult `json:"syncResult"`
}

type operation struct {
	InitiatedBy initiatedBy `json:"initiatedBy"`
}

type initiatedBy struct {
	Username  string `json:"username"`
	Automated bool   `json:"automated"`
}

type syncResult struct {
	Revision  string           `json:"revision"`
	Resources []resourceResult `json:"resources"`
}

type resourceResult struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type resourceStatus struct {
	Kind      string        `json:"kind"`
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Status    string        `json:"status"`
	Health    *healthStatus `json:"health"`
}

type revisionHistory struct {
	Revision   string     `json:"revision"`
	DeployedAt *time.Time `json:"deployedAt"`
}

type applicationCond struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type resourceTree struct {
	Nodes []resourceNode `json:"nodes"`
}

type resourceNode struct {
	Kind      string        `json:"kind"`
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Health    *healthStatus `json:"health"`
}

type revisionMetadata struct {
	Author  string     `json:"author"`
	Date    *time.Time `json:"date"`
	Message string     `json:"message"`
}

type argoError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func (a application) source() applicationSrc {
	if a.Spec.Source != nil {
		return *a.Spec.Source
	}
	if len(a.Spec.Sources) > 0 {
		return a.Spec.Sources[0]
	}
	return applicationSrc{}
}

func (a application) cluster() string {
	if a.Spec.Destination.Name != "" {
		return a.Spec.Destination.Name
	}
	return a.Spec.Destination.Server
}

func (a application) phase() string {
	if a.Status.OperationState == nil {
		return ""
	}
	return a.Status.OperationState.Phase
}

func (a application) initiatedBy() string {
	if a.Status.OperationState == nil {
		return ""
	}
	by := a.Status.OperationState.Operation.InitiatedBy
	if by.Username != "" {
		return by.Username
	}
	if by.Automated {
		return "automated"
	}
	return ""
}

func (a application) latestHistory() *revisionHistory {
	if len(a.Status.History) == 0 {
		return nil
	}
	return &a.Status.History[len(a.Status.History)-1]
}
