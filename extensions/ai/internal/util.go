package internal

func ConvertToFloatPtrSlice(input []float32) []*float32 {
	result := make([]*float32, len(input))
	for i := range input {
		result[i] = &input[i]
	}

	return result
}
