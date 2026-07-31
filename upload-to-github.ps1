# ============================================================
# TokenMonitor - GitHub 上传脚本
# ============================================================
# 使用方法:
#   1. 在 PowerShell 中 cd 到项目目录:  cd E:\Code\TokenMonitor
#   2. 编辑本文件，把下面的变量填上你的信息
#   3. 执行:  powershell -ExecutionPolicy Bypass -File .\upload-to-github.ps1
# ============================================================

# ====== 在这里填入你的信息 ======
$GITHUB_USERNAME = "YuanTutu"             # 你的 GitHub 用户名
$GITHUB_EMAIL    = "944947973@qq.com"     # 你的 GitHub 邮箱
$GITHUB_TOKEN    = "在这里粘贴你的Personal Access Token"  # 例: ghp_xxxxxxxxxxxx
$REPO_NAME       = "TokenMonitor"         # 仓库名，可改
$REPO_DESC       = "命令行 TUI 工具，集中监控多个 AI 平台的套餐余额和用量"
$IS_PRIVATE      = $false                 # $false = 公开, $true = 私有
# ================================

$ErrorActionPreference = "Stop"

Write-Host "=== TokenMonitor GitHub 上传脚本 ===" -ForegroundColor Cyan

# 1. 检查 git 是否可用
Write-Host "`n[1/6] 检查 git..." -ForegroundColor Yellow
$gitVersion = git --version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ git 未安装或不在 PATH 中" -ForegroundColor Red
    Write-Host "   请从 https://git-scm.com/download/win 安装 Git" -ForegroundColor Red
    exit 1
}
Write-Host "✅ $gitVersion"

# 配置 git 提交者信息
git config user.name $GITHUB_USERNAME
git config user.email $GITHUB_EMAIL
Write-Host "✅ git 提交者: $GITHUB_USERNAME <$GITHUB_EMAIL>"

# 2. 检查当前目录是否已经是 git 仓库
Write-Host "`n[2/6] 初始化 git 仓库..." -ForegroundColor Yellow
if (Test-Path ".git") {
    Write-Host "⚠️  .git 目录已存在，将重新初始化"
    Remove-Item -Recurse -Force ".git"
}
git init
git branch -M main

# 3. 创建 GitHub 仓库
Write-Host "`n[3/6] 创建 GitHub 仓库..." -ForegroundColor Yellow
$apiUrl = "https://api.github.com/user/repos"
$body = @{
    name = $REPO_NAME
    description = $REPO_DESC
    private = $IS_PRIVATE
    auto_init = $false
} | ConvertTo-Json

$headers = @{
    "Authorization" = "Bearer $GITHUB_TOKEN"
    "Accept" = "application/vnd.github+json"
    "X-GitHub-Api-Version" = "2022-11-28"
}

try {
    $response = Invoke-RestMethod -Uri $apiUrl -Method Post -Headers $headers -Body $body -ContentType "application/json"
    Write-Host "✅ 仓库已创建: $($response.html_url)"
    $repoUrl = $response.html_url
    $cloneUrl = "https://$GITHUB_USERNAME`:$GITHUB_TOKEN@github.com/$GITHUB_USERNAME/$REPO_NAME.git"
} catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    if ($statusCode -eq 422) {
        Write-Host "⚠️  仓库可能已存在，尝试使用现有仓库..." -ForegroundColor Yellow
        $cloneUrl = "https://$GITHUB_USERNAME`:$GITHUB_TOKEN@github.com/$GITHUB_USERNAME/$REPO_NAME.git"
    } else {
        Write-Host "❌ 创建仓库失败: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "   HTTP 状态码: $statusCode" -ForegroundColor Red
        exit 1
    }
}

# 4. 添加文件并提交
Write-Host "`n[4/6] 添加文件并提交..." -ForegroundColor Yellow
git add -A

# 显示将要提交的文件
Write-Host "`n将要提交的文件:" -ForegroundColor DarkGray
git status --short

git commit -m "Initial commit: TokenMonitor TUI 工具

- 支持智谱 GLM、火山方舟、DeepSeek 三个平台
- 卡片式 TUI 界面 + 进度条可视化
- 火山引擎 V4 签名算法自实现
- TUI 内配置管理 + 自动刷新 + 重置提醒"

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ git commit 失败" -ForegroundColor Red
    exit 1
}

# 5. 推送到 GitHub
Write-Host "`n[5/6] 推送到 GitHub..." -ForegroundColor Yellow
git remote add origin $cloneUrl
git push -u origin main

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ 推送失败" -ForegroundColor Red
    Write-Host "   请检查用户名和 Token 是否正确" -ForegroundColor Red
    exit 1
}

# 6. 完成
Write-Host "`n[6/6] 完成!" -ForegroundColor Green
Write-Host "`n✅ 代码已上传到 GitHub!" -ForegroundColor Green
Write-Host "   仓库地址: https://github.com/$GITHUB_USERNAME/$REPO_NAME" -ForegroundColor Cyan

# 清理：从 git remote 中移除 token（安全起见）
git remote set-url origin "https://github.com/$GITHUB_USERNAME/$REPO_NAME.git"
Write-Host "`n🔒 已从 remote URL 中移除 token" -ForegroundColor DarkGray

Write-Host "`n提示: 上传完成后建议删除本脚本 (upload-to-github.ps1) 以避免泄露 token" -ForegroundColor Yellow
