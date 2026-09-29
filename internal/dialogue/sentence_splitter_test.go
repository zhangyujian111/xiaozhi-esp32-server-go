package dialogue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentenceSplitter_NoDelimiter_NoOutput(t *testing.T) {
	splitter := NewSentenceSplitter()
	require.NotNil(t, splitter)

	result := splitter.Feed("hello")
	require.NotNil(t, result)
	assert.Empty(t, result)

	remainder := splitter.Flush()
	assert.Equal(t, "hello", remainder)
}

func TestSentenceSplitter_SingleSentence(t *testing.T) {
	splitter := NewSentenceSplitter()

	result := splitter.Feed("你好")
	require.NotNil(t, result)
	assert.Empty(t, result)

	result = splitter.Feed("。")
	require.NotNil(t, result)
	assert.Equal(t, []string{"你好。"}, result)

	remainder := splitter.Flush()
	assert.Empty(t, remainder)
}

func TestSentenceSplitter_MultipleDelimiters_SplitAtLast(t *testing.T) {
	splitter := NewSentenceSplitter()

	splitter.Feed("第一")
	splitter.Feed("句")
	splitter.Feed("。")
	result := splitter.Feed("第二")
	splitter.Feed("句")
	result = splitter.Feed("！")

	assert.Equal(t, []string{"第二句！"}, result)
}

func TestSentenceSplitter_MixedDelimiters(t *testing.T) {
	splitter := NewSentenceSplitter()

	splitter.Feed("问")
	splitter.Feed("题")
	splitter.Feed("？")
	result := splitter.Feed("答案")
	splitter.Feed("!")

	assert.Equal(t, []string{"答案!"}, result)
}

func TestSentenceSplitter_FlushReturnsRemainder(t *testing.T) {
	splitter := NewSentenceSplitter()

	splitter.Feed("没有")
	splitter.Feed("句末")
	splitter.Feed("标点")

	remainder := splitter.Flush()
	assert.Equal(t, "没有句末标点", remainder)

	remainder = splitter.Flush()
	assert.Empty(t, remainder)
}

func TestSentenceSplitter_ConsecutiveFeeds(t *testing.T) {
	splitter := NewSentenceSplitter()

	result := splitter.Feed("第")
	result = splitter.Feed("一")
	result = splitter.Feed("句")
	result = splitter.Feed("。")
	assert.Equal(t, []string{"第一句。"}, result)

	result = splitter.Feed("第")
	result = splitter.Feed("二")
	result = splitter.Feed("句")
	result = splitter.Feed("！")
	assert.Equal(t, []string{"第二句！"}, result)

	remainder := splitter.Flush()
	assert.Empty(t, remainder)
}
