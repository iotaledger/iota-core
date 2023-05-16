package promise

type Bool struct {
	*Value[bool]
}

func (b *Bool) Unset() {}

func (b *Bool) Set() {}

func (b *Bool) Toggle() {}
