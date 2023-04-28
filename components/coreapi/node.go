package coreapi

//nolint:unparam // we have no error case right now
func info() (*infoResponse, error) {
	return &infoResponse{
		Name:    deps.AppInfo.Name,
		Version: deps.AppInfo.Version,
		Status:  nodeStatus{},
	}, nil
}
