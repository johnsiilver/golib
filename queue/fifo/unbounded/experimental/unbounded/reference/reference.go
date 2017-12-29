package reference

type Buffer struct {
	Data chan interface{}
}

func (b *Buffer) Force(item interface{}) {
	b.Data <- item
}

func (b *Buffer) Pull() interface{} {
	return <-b.Data
}
