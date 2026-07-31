package state

// Store persists rename intentions. Implementations must not retain caller-owned slices.
type Store interface {
	Load() ([]Intent, error)
	Save([]Intent) error
}
