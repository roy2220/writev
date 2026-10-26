package writev

const DefaultMaxBatchSize = defaultMaxBatchSize

func (w Writer) WriteBuffersFast(buffers [][]byte) (int, error) {
	return w.writeBuffersFast(buffers)
}

func (w Writer) WriteBuffersSlow(buffers [][]byte, maxBatchSize int) (int, error) {
	return w.writeBuffersSlow(buffers, maxBatchSize)
}
