package votes

func IsThresholdReached(objectWeight, totalWeight int64, threshold float64) bool {
	if totalWeight == 0 {
		return false
	}

	return objectWeight > int64(float64(totalWeight)*threshold)
}
