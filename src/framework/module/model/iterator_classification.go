package model

var _ ClassificationIterator = (*classificationIterator)(nil)

type classificationIterator struct {
}

func newClassificationIterator() (ClassificationIterator, error) {
	// TODO: 在实例化的时候默认查询一定数量的Classification，每次调用Next返回数据中的一个，当读取到缓存的数据的最后一条后开始重新组织数据，直到将数据库里的数据全部读取完毕
	return nil, nil
}

func (cli *classificationIterator) Next() (Classification, error) {
	return nil, nil
}
