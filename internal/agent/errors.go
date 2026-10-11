package agent

import (
	"context"
	"errors"

	"glance/internal/agent/provider"
)

func DescribeTurnError(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "cancelled", "请求已停止。"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout", "模型请求超时，可以检查服务状态后重试。"
	case errors.Is(err, ErrPromptTooLarge), errors.Is(err, ErrMessageTooLarge), errors.Is(err, ErrToolArgumentsTooLarge), errors.Is(err, ErrToolResultTooLarge):
		return "agent_limit", "本轮对话或工具结果超过 Glance 的处理上限，请缩小输入范围后重试。"
	case errors.Is(err, ErrToolCallLimit), errors.Is(err, ErrToolRoundLimit):
		return "agent_limit", "本轮工具调用次数达到上限；可以把任务拆成更小的步骤。"
	case errors.Is(err, ErrEmptyModelResponse):
		return "invalid_response", "模型没有返回可用内容，请检查模型配置后重试。"
	}
	category, ok := provider.ClassifyError(err)
	if !ok {
		return "agent_error", "Agent 操作失败：" + err.Error()
	}
	switch category {
	case provider.ErrorUnconfigured:
		return string(category), "模型配置不完整，请检查服务地址、模型名和 API Key。"
	case provider.ErrorAuthentication:
		return string(category), "模型服务认证失败，请检查 API Key。"
	case provider.ErrorRateLimited:
		return string(category), "模型服务当前限流，请稍后重试。"
	case provider.ErrorTimeout:
		return string(category), "模型请求超时，可以检查服务状态后重试。"
	case provider.ErrorNetwork:
		return string(category), "无法连接模型服务，请检查网络和服务地址。"
	case provider.ErrorInvalidRequest:
		return string(category), "模型服务拒绝了请求，请检查模型名和服务兼容性。"
	case provider.ErrorInvalidResponse:
		return string(category), "模型服务返回了无法识别的响应。"
	case provider.ErrorService:
		return string(category), "模型服务暂时不可用，请稍后重试。"
	default:
		return "provider_error", "模型调用失败，请检查模型设置后重试。"
	}
}
