package coreapi

func info() (*infoResponse, error) {
	return &infoResponse{
		Name:    deps.AppInfo.Name,
		Version: deps.AppInfo.Version,
		Status:  nodeStatus{},
	}, nil

}
