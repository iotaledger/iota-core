package votes

func IsThresholdReached(objectWeight, totalWeight int, threshold float64) bool {
	if totalWeight == 0 {
		return false
	}

	return objectWeight > int(float64(totalWeight)*threshold)
}
