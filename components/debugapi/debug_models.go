package debugapi

type (
	ValidatorsSummaryResponse struct {
		ValidatorSeats map[int]string `json:"validatorSeats"`
		ActiveSeats    []int          `json:"activeSeats"`
	}
)
