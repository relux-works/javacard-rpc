package codegen

func fixedMessageLength(fields []Field) (int, bool) {
	total := 0
	for _, field := range fields {
		size, fixed := field.WireSize()
		if !fixed {
			return 0, false
		}
		total += size
	}
	return total, true
}

func responseFields(msg *Message) []Field {
	if msg == nil {
		return nil
	}
	return msg.Fields
}
