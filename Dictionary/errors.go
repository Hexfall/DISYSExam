package dictionary

type ImpermissibleError struct{}

func (i *ImpermissibleError) Error() string {
	return "Replica lacks permission to perform this action."
}
