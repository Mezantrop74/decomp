package main

func main(_0 int32, _1 **int8) int32 {
	_3 = new(int32)
	_4 = new(int32)
	_5 = new(**int8)
	_6 = new(int32)
	_7 = new(int32)
	*_3 = 0
	*_4 = _0
	*_5 = _1
	*_7 = 0
	*_6 = 0
	_9 = *_6
	_10 = _9 < 10
	for _10 {
		_12 = *_7
		_13 = _12 < 100
		if _13 {
			_15 = *_6
			_16 = 3 * _15
			_17 = *_7
			_18 = _17 + _16
			*_7 = _18
		}
		_21 = *_6
		_22 = _21 + 1
		*_6 = _22
	}
	_24 = *_7
	return _24
}
