package api

// Driver is mutli implement interface
type Driver interface {
	// List applications
	List(envName string) ([]*Application, error)
	// Save application
	Save(app *Application, envName string) error
	// Delete application
	Delete(envName, appName string) error
	// Get application
	Get(envName, appName string) (*Application, error)
	// Name of storage driver
	Name() string
}
