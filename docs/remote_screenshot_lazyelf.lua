-- AutoGo 远程截图客户端示例
-- 只需要把 SERVER_URL 改成内网穿透地址。
-- 本协议不需要 Token、注册、心跳、会话 ID 或分片。

local SERVER_URL = "http://154.219.109.245:15009"
local SCREENSHOT_PATH = "/mnt/sdcard/autogo_remote_screenshot.png"
local POLL_WAIT_SECONDS = 20
local HTTP_TIMEOUT_SECONDS = 30

local function headers()
    return {
        ["Content-Type"] = "application/json; charset=UTF-8",
        ["Accept"] = "application/json"
    }
end

local function postJson(url, value)
    local body = jsonLib.encode(value)
    local requestHeaders = headers()

    -- net.http.post 的第二个参数必须是原始请求字符串，不能传 Map。
    -- 如果你的懒人精灵版本提供 net.http.post，请使用这个调用。
    if type(net) == "table" and type(net.http) == "table" and type(net.http.post) == "function" then
        local ok, response, code = pcall(
            net.http.post,
            url,
            body,
            requestHeaders,
            HTTP_TIMEOUT_SECONDS
        )
        if ok and response ~= nil then
            return response, code
        end
        if not ok then
            print("AutoGo HTTP POST 调用异常: " .. tostring(response))
        else
            print("AutoGo HTTP POST 没有返回内容")
        end
    end

    -- 某些版本把原始 JSON POST 暴露为 httpPostJson。
    if type(httpPostJson) == "function" then
        return httpPostJson(url, body, requestHeaders, HTTP_TIMEOUT_SECONDS)
    end

    error("当前懒人精灵没有可发送原始 JSON 的 HTTP POST 接口")
end

local function decodeResponse(body)
    if body == nil or body == "" then
        return nil
    end
    local ok, value = pcall(jsonLib.decode, body)
    if not ok then
        print("AutoGo JSON 解析失败: " .. tostring(value) .. ", body=" .. tostring(body))
        return nil
    end
    return value
end

local function screenshotAndReadBase64()
    local ok = snapShot(SCREENSHOT_PATH)
    print("snapShot 返回: " .. tostring(ok))
    sleep(300)

    local value = getFileBase64(SCREENSHOT_PATH)
    value = tostring(value or "")
    local marker = "base64,"
    local markerPos = string.find(value, marker, 1, true)
    if markerPos ~= nil then
        value = string.sub(value, markerPos + string.len(marker))
    end
    print("截图 Base64 长度: " .. tostring(string.len(value)))
    return value
end

local function uploadScreenshot()
    local encoded = screenshotAndReadBase64()
    if encoded == "" then
        print("读取截图 Base64 失败: " .. SCREENSHOT_PATH)
        return false
    end

    local body, code = postJson(SERVER_URL .. "/api/remote/screenshot", {
        code = 0,
        message = "success",
        data = { base64 = encoded }
    })
    local result = decodeResponse(body)
    if result ~= nil and result.code == 0 then
        print("AutoGo 截图上传成功")
        return true
    end
    print("AutoGo 截图上传失败, http_code=" .. tostring(code) .. ", body=" .. tostring(body))
    return false
end

print("AutoGo 远程截图轮询启动: " .. SERVER_URL)

while true do
    local body, code = postJson(
        SERVER_URL .. "/api/remote/task?wait=" .. tostring(POLL_WAIT_SECONDS),
        { code = 0, message = "success", data = {} }
    )
    local command = decodeResponse(body)

    if command ~= nil and command.code == 0 and command.Task == "Screenshot" then
        uploadScreenshot()
    elseif command == nil then
        print("AutoGo 轮询失败, http_code=" .. tostring(code) .. ", body=" .. tostring(body))
        sleep(1000)
    end
end
