package api

// Driver is mutli implement interface
type Driver interface {
	// Save application
	Save(app *Application, envName string) error
	// Delete application
	Delete(envName, appName string) error
	// Name of storage driver
	Name() string
}
