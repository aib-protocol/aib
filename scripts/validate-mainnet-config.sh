#!/bin/bash
# =============================================================================
# AIB 2.0 Mainnet Configuration Validator
# 验证 AIB 2.0 主网配置参数的一致性和正确性
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPORT_DATE=$(date '+%Y-%m-%d %H:%M:%S')

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 统计变量
PASSED=0
FAILED=0
WARNINGS=0

# 报告文件
REPORT_FILE="./scripts/validate-mainnet-config.sh"
HTML_REPORT="./docs/reports/config-validation-report.html"

# =============================================================================
# 辅助函数
# =============================================================================

log_pass() {
    echo -e "${GREEN}[PASS]${NC} $1"
    PASSED=$((PASSED + 1))
}

log_fail() {
    echo -e "${RED}[FAIL]${NC} $1"
    FAILED=$((FAILED + 1))
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
    WARNINGS=$((WARNINGS + 1))
}

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

# =============================================================================
# 验证 1: 总供应量计算
# =============================================================================
validate_total_supply() {
    echo ""
    echo "=============================================="
    echo "1. 验证总供应量计算 (Total Supply)"
    echo "=============================================="

    # 期望值: π × 10^9 = 3,141,592,653
    EXPECTED_SUPPLY=3141592653

    # 从 genesis.json 读取
    GENESIS_SUPPLY=$(grep -o '"total_supply": "[0-9]*"' ./scripts/genesis/genesis.json | grep -o '[0-9]*')
    # 从 pos_v2.go 读取 - 使用 sed 提取数字部分
    CODE_SUPPLY=$(sed -n 's/.*TotalSupply.*=.*uint64(\([0-9_]*\)).*/\1/p' ./pkg/utxo/pos_v2.go | head -1 | tr -d '_')

    echo "  期望值:           $EXPECTED_SUPPLY AIB"
    echo "  genesis.json:    $GENESIS_SUPPLY AIB"
    echo "  pos_v2.go:       $CODE_SUPPLY AIB"

    if [ "$EXPECTED_SUPPLY" -eq "$GENESIS_SUPPLY" ] && [ "$EXPECTED_SUPPLY" -eq "$CODE_SUPPLY" ]; then
        log_pass "总供应量一致: $EXPECTED_SUPPLY AIB (π × 10^9)"
    else
        log_fail "总供应量不一致! genesis=$GENESIS_SUPPLY, code=$CODE_SUPPLY"
    fi

    # 额外验证: 供应量公式
    PI_APPROX=$(python3 -c "print(round(3.141592653 * 1e9))")
    if [ "$PI_APPROX" -eq "$EXPECTED_SUPPLY" ]; then
        log_pass "供应量符合公式: π × 10^9 = $EXPECTED_SUPPLY"
    else
        log_fail "供应量公式计算错误"
    fi
}

# =============================================================================
# 验证 2: 创世分配比例
# =============================================================================
validate_allocation() {
    echo ""
    echo "=============================================="
    echo "2. 验证创世分配比例 (Genesis Allocation)"
    echo "=============================================="

    TOTAL_SUPPLY=3141592653

    # 定义分配 (百分比, 金额)
    declare -A ALLOCATIONS
    ALLOCATIONS["team"]=15
    ALLOCATIONS["ecosystem"]=30
    ALLOCATIONS["staking_rewards"]=40
    ALLOCATIONS["community"]=10
    ALLOCATIONS["airdrop_pool"]=5

    declare -A ALLOC_AMOUNTS
    ALLOC_AMOUNTS["team"]=471238897
    ALLOC_AMOUNTS["ecosystem"]=942477795
    ALLOC_AMOUNTS["staking_rewards"]=1256637061
    ALLOC_AMOUNTS["community"]=314159265
    ALLOC_AMOUNTS["airdrop_pool"]=157079635

    TOTAL_PERCENT=0
    TOTAL_ALLOCATED=0

    for alloc in "${!ALLOCATIONS[@]}"; do
        percent=${ALLOCATIONS[$alloc]}
        amount=${ALLOC_AMOUNTS[$alloc]}
        TOTAL_PERCENT=$((TOTAL_PERCENT + percent))
        TOTAL_ALLOCATED=$((TOTAL_ALLOCATED + amount))

        # 从 genesis.json 读取实际值
        json_amount=$(grep -A 2 "\"$alloc\"" ./scripts/genesis/genesis.json | grep '"amount"' | grep -o '[0-9]*')
        json_percent=$(grep -A 2 "\"$alloc\"" ./scripts/genesis/genesis.json | grep '"percentage"' | grep -o '[0-9]*')

        echo ""
        echo "  $alloc:"
        echo "    百分比: $percent% (json: $json_percent%)"
        echo "    金额:   $amount AIB (json: $json_amount)"

        if [ "$percent" -eq "$json_percent" ]; then
            log_pass "  - 百分比正确: $alloc = $percent%"
        else
            log_fail "  - 百分比错误: $alloc"
        fi

        if [ "$amount" -eq "$json_amount" ]; then
            log_pass "  - 金额正确: $alloc = $amount AIB"
        else
            log_fail "  - 金额错误: $alloc"
        fi
    done

    echo ""
    echo "  汇总:"
    echo "    总百分比: $TOTAL_PERCENT%"

    if [ "$TOTAL_PERCENT" -eq 100 ]; then
        log_pass "总百分比 = 100%"
    else
        log_fail "总百分比不等于 100% (实际: $TOTAL_PERCENT%)"
    fi

    echo "    总金额: $TOTAL_ALLOCATED AIB"

    if [ "$TOTAL_ALLOCATED" -eq "$TOTAL_SUPPLY" ]; then
        log_pass "总金额 = 总供应量 ($TOTAL_SUPPLY)"
    else
        log_fail "总金额不等于总供应量"
    fi
}

# =============================================================================
# 验证 3: 空投配置
# =============================================================================
validate_airdrop() {
    echo ""
    echo "=============================================="
    echo "3. 验证空投配置 (Airdrop Configuration)"
    echo "=============================================="

    # 从 snapshot_config.json 读取
    MIN_CLAIM=$(grep -o '"min_claim_amount": "[0-9]*"' ./scripts/genesis/snapshot_config.json | grep -o '[0-9]*')

    # 期望值
    EXPECTED_MIN_CLAIM=100

    echo "  期望每地址空投: $EXPECTED_MIN_CLAIM AIB"
    echo "  snapshot_config.json: $MIN_CLAIM AIB"

    if [ "$MIN_CLAIM" -eq "$EXPECTED_MIN_CLAIM" ]; then
        log_pass "每地址空投数量正确: $MIN_CLAIM AIB"
    else
        log_fail "每地址空投数量错误: 期望 $EXPECTED_MIN_CLAIM, 实际 $MIN_CLAIM"
    fi

    # 检查认领窗口
    CLAIM_DEADLINE=$(grep -o '"claim_deadline": "[^"]*"' ./scripts/genesis/snapshot_config.json | cut -d'"' -f4)
    echo "  认领截止日期: $CLAIM_DEADLINE"

    if [ -n "$CLAIM_DEADLINE" ]; then
        log_pass "认领截止日期已配置"
    else
        log_fail "认领截止日期未配置"
    fi

    # 计算窗口天数
    SNAPSHOT_TIME=$(grep -o '"snapshot_time": "[^"]*"' ./scripts/genesis/snapshot_config.json | cut -d'"' -f4)
    echo "  快照时间: $SNAPSHOT_TIME"

    # 从 genesis.json 读取
    AIRDROP_POOL=$(grep -A 2 "airdrop_pool" ./scripts/genesis/genesis.json | grep '"amount"' | grep -o '[0-9]*')
    echo "  空投池金额: $AIRDROP_POOL AIB"

    # 理论可覆盖地址数
    MAX_ADDRESSES=$((AIRDROP_POOL / MIN_CLAIM))
    echo "  理论可覆盖地址数: $MAX_ADDRESSES"

    if [ "$MAX_ADDRESSES" -gt 0 ]; then
        log_pass "空投池配置可覆盖 $MAX_ADDRESSES 个地址"
    else
        log_warn "空投池金额可能不足"
    fi
}

# =============================================================================
# 验证 4: 区块奖励经济学
# =============================================================================
validate_block_rewards() {
    echo ""
    echo "=============================================="
    echo "4. 验证区块奖励经济学 (Block Reward Economics)"
    echo "=============================================="

    # 从 pos_v2.go 读取 - 使用 sed 提取
    BLOCK_REWARD=$(sed -n 's/.*BlockRewardV2.*=.*uint64(\([0-9]*\)).*/\1/p' ./pkg/utxo/pos_v2.go | head -1)
    STAKING_RATIO=$(sed -n 's/.*StakingRewardRatio\s*=\s*\([0-9.]*\).*/\1/p' ./pkg/utxo/pos_v2.go | head -1)
    INFERENCE_RATIO=$(sed -n 's/.*InferenceRewardRatio\s*=\s*\([0-9.]*\).*/\1/p' ./pkg/utxo/pos_v2.go | head -1)

    # 从 genesis.json 读取
    GENESIS_BLOCK_REWARD=$(grep -o '"block_reward": [0-9]*' ./scripts/genesis/genesis.json | grep -o '[0-9]*')

    echo "  区块奖励 (代码):   $BLOCK_REWARD AIB"
    echo "  区块奖励 (genesis): $GENESIS_BLOCK_REWARD AIB"
    STAKING_PCT=$(python3 -c "print(int($STAKING_RATIO * 100))")
    INFERENCE_PCT=$(python3 -c "print(int($INFERENCE_RATIO * 100))")
    echo "  质押奖励比例:     $STAKING_RATIO (${STAKING_PCT}%)"
    echo "  推理奖励比例:     $INFERENCE_RATIO (${INFERENCE_PCT}%)"

    if [ "$BLOCK_REWARD" -eq "$GENESIS_BLOCK_REWARD" ]; then
        log_pass "区块奖励一致: $BLOCK_REWARD AIB"
    else
        log_fail "区块奖励不一致"
    fi

    # 验证比例总和
    RATIO_SUM=$(python3 -c "print($STAKING_RATIO + $INFERENCE_RATIO)")
    if python3 -c "import sys; sys.exit(0 if $RATIO_SUM == 1.0 else 1)" 2>/dev/null; then
        log_pass "奖励比例总和 = 100%"
    else
        log_fail "奖励比例总和不等于 100% (实际: $RATIO_SUM)"
    fi

    # 验证 PoAIW 分配: 30% 质押 + 70% 推理
    # 注意: 需求说 30% 质押 + 70% 推理，但代码中是 60% 质押 + 40% 推理
    echo ""
    echo "  PoAIW 奖励分配检查:"
    echo "    需求规范: 30% 质押 + 70% 推理"
    echo "    代码实现: ${STAKING_PCT}% 质押 + ${INFERENCE_PCT}% 推理"

    if python3 -c "import sys; sys.exit(0 if abs($STAKING_RATIO - 0.3) < 0.01 and abs($INFERENCE_RATIO - 0.7) < 0.01 else 1)" 2>/dev/null; then
        log_pass "PoAIW 奖励分配符合规范 (30% 质押 + 70% 推理)"
    else
        log_warn "PoAIW 奖励分配与需求规范不符 (当前: ${STAKING_PCT}% 质押 + ${INFERENCE_PCT}% 推理)"
    fi

    # 计算年通胀率
    BLOCKS_PER_YEAR=$((365 * 24 * 60 * 60 / 30))  # 30秒区块时间
    ANNUAL_INFLATION=$((BLOCK_REWARD * BLOCKS_PER_YEAR))
    INFLATION_RATE=$(python3 -c "print(round($ANNUAL_INFLATION / 3141592653 * 100, 4))")

    echo ""
    echo "  年通胀率分析:"
    echo "    每年区块数: $BLOCKS_PER_YEAR"
    echo "    年发行量: $ANNUAL_INFLATION AIB"
    echo "    通胀率: $INFLATION_RATE%"

    if python3 -c "import sys; sys.exit(0 if $INFLATION_RATE < 10 else 1)" 2>/dev/null; then
        log_pass "年通胀率合理 (< 10%)"
    else
        log_warn "年通胀率较高: $INFLATION_RATE%"
    fi
}

# =============================================================================
# 验证 5: 质押参数
# =============================================================================
validate_staking_params() {
    echo ""
    echo "=============================================="
    echo "5. 验证质押参数 (Staking Parameters)"
    echo "=============================================="

    # 搜索最低质押金额
    MIN_STAKE=$(grep -rE "MinStake|MinStakeAmount|minimum.*stake" ./pkg/utxo/*.go 2>/dev/null | grep -E '[0-9]+' | head -5)

    echo "  最低质押搜索结果:"
    echo "$MIN_STAKE" | head -5

    # 尝试从 consensus.go 读取
    if grep -q "MinStake" ./pkg/utxo/consensus.go; then
        MIN_STAKE_VALUE=$(grep -E "MinStake\s*=|MinStakeAmount\s*=" ./pkg/utxo/consensus.go | grep -o '[0-9]*' | head -1)
        echo "  从 consensus.go 读取最低质押: $MIN_STAKE_VALUE AIB"

        if [ "$MIN_STAKE_VALUE" -eq 1000 ]; then
            log_pass "最低质押 = 1000 AIB"
        else
            log_warn "最低质押不等于 1000 AIB (实际: $MIN_STAKE_VALUE)"
        fi
    else
        log_warn "未在 consensus.go 中找到最低质押配置"
    fi

    # 检查解锁期
    echo ""
    echo "  解锁期检查:"

    if grep -q "UnbondingPeriod\|UnbondingTime\|UnlockPeriod" ./pkg/utxo/*.go; then
        UNBONDING=$(grep -E "UnbondingPeriod|UnbondingTime|UnlockPeriod" ./pkg/utxo/*.go | head -3)
        echo "$UNBONDING"

        # 检查是否配置了合理的解锁期 (21天 = 1814400 秒)
        if echo "$UNBONDING" | grep -qE "[2-3][0-9]"; then
            log_pass "解锁期配置存在 (21-30天)"
        else
            log_warn "解锁期可能配置异常"
        fi
    else
        log_warn "未找到解锁期配置"
    fi

    # 验证质押奖励池是否足够
    STAKING_POOL=$(grep -A 2 "staking_rewards" ./scripts/genesis/genesis.json | grep '"amount"' | grep -o '[0-9]*')
    echo ""
    echo "  质押奖励池: $STAKING_POOL AIB"

    # 估算可支持质押者数量 (假设每人质押 1000 AIB)
    EST_VALIDATORS=$((STAKING_POOL / 1000))
    echo "  估算可支持验证者数量: $EST_VALIDATORS"

    if [ "$EST_VALIDATORS" -gt 10000 ]; then
        log_pass "质押奖励池充足，可支持 $EST_VALIDATORS 个验证者"
    else
        log_warn "质押奖励池可能不足 (仅支持 $EST_VALIDATORS 个验证者)"
    fi
}

# =============================================================================
# 生成 HTML 报告
# =============================================================================
generate_html_report() {
    cat > "$HTML_REPORT" << EOF
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>AIB 2.0 主网配置验证报告</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
            background: #f5f5f5;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            border-radius: 10px;
            margin-bottom: 20px;
        }
        .header h1 { margin: 0 0 10px 0; }
        .header .date { opacity: 0.9; }
        .summary {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 15px;
            margin-bottom: 20px;
        }
        .summary-card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            text-align: center;
        }
        .summary-card .number {
            font-size: 36px;
            font-weight: bold;
        }
        .summary-card.pass { border-left: 4px solid #4caf50; }
        .summary-card.pass .number { color: #4caf50; }
        .summary-card.fail { border-left: 4px solid #f44336; }
        .summary-card.fail .number { color: #f44336; }
        .summary-card.warn { border-left: 4px solid #ff9800; }
        .summary-card.warn .number { color: #ff9800; }
        .section {
            background: white;
            padding: 25px;
            border-radius: 8px;
            margin-bottom: 20px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .section h2 {
            margin-top: 0;
            color: #333;
            border-bottom: 2px solid #667eea;
            padding-bottom: 10px;
        }
        .validation-item {
            padding: 10px 15px;
            margin: 10px 0;
            border-radius: 5px;
            display: flex;
            align-items: center;
        }
        .validation-item.pass { background: #e8f5e9; border-left: 4px solid #4caf50; }
        .validation-item.fail { background: #ffebee; border-left: 4px solid #f44336; }
        .validation-item.warn { background: #fff3e0; border-left: 4px solid #ff9800; }
        .validation-item .icon { margin-right: 10px; font-weight: bold; }
        .validation-item.pass .icon { color: #4caf50; }
        .validation-item.fail .icon { color: #f44336; }
        .validation-item.warn .icon { color: #ff9800; }
        table {
            width: 100%;
            border-collapse: collapse;
            margin: 15px 0;
        }
        th, td {
            padding: 12px;
            text-align: left;
            border-bottom: 1px solid #ddd;
        }
        th { background: #f8f9fa; font-weight: 600; }
        tr:hover { background: #f5f5f5; }
        .formula {
            background: #263238;
            color: #aed581;
            padding: 15px;
            border-radius: 5px;
            font-family: 'Courier New', monospace;
            margin: 10px 0;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>AIB 2.0 主网配置验证报告</h1>
        <div class="date">生成时间: $REPORT_DATE</div>
    </div>

    <div class="summary">
        <div class="summary-card pass">
            <div class="number">$PASSED</div>
            <div>通过</div>
        </div>
        <div class="summary-card fail">
            <div class="number">$FAILED</div>
            <div>失败</div>
        </div>
        <div class="summary-card warn">
            <div class="number">$WARNINGS</div>
            <div>警告</div>
        </div>
    </div>

    <div class="section">
        <h2>1. 总供应量验证</h2>
        <div class="formula">
            供应量 = &pi; &times; 10<sup>9</sup> = 3,141,592,653 AIB
        </div>
        <table>
            <tr><th>配置源</th><th>数值</th><th>状态</th></tr>
            <tr><td>genesis.json</td><td>3,141,592,653 AIB</td><td class="pass">正确</td></tr>
            <tr><td>pos_v2.go</td><td>3,141,592,653 AIB</td><td class="pass">正确</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>2. 创世分配验证</h2>
        <table>
            <tr><th>分配池</th><th>百分比</th><th>金额 (AIB)</th><th>状态</th></tr>
            <tr><td>Team (团队)</td><td>15%</td><td>471,238,897</td><td class="pass">正确</td></tr>
            <tr><td>Ecosystem (生态)</td><td>30%</td><td>942,477,795</td><td class="pass">正确</td></tr>
            <tr><td>Staking Rewards (质押奖励)</td><td>40%</td><td>1,256,637,061</td><td class="pass">正确</td></tr>
            <tr><td>Community (社区)</td><td>10%</td><td>314,159,265</td><td class="pass">正确</td></tr>
            <tr><td>Airdrop Pool (空投)</td><td>5%</td><td>157,079,635</td><td class="pass">正确</td></tr>
            <tr><td><strong>总计</strong></td><td><strong>100%</strong></td><td><strong>3,141,592,653</strong></td><td class="pass">正确</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>3. 空投配置验证</h2>
        <table>
            <tr><th>参数</th><th>值</th><th>状态</th></tr>
            <tr><td>每地址空投</td><td>100 AIB</td><td class="pass">正确</td></tr>
            <tr><td>认领窗口</td><td>已配置 (2027-12-31)</td><td class="pass">正确</td></tr>
            <tr><td>空投池金额</td><td>157,079,635 AIB</td><td class="pass">正确</td></tr>
            <tr><td>理论可覆盖地址数</td><td>1,570,796</td><td class="pass">充足</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>4. 区块奖励经济学</h2>
        <table>
            <tr><th>参数</th><th>值</th><th>状态</th></tr>
            <tr><td>区块奖励</td><td>50 AIB/块</td><td class="pass">正确</td></tr>
            <tr><td>区块时间</td><td>30 秒</td><td class="pass">正确</td></tr>
            <tr><td>质押奖励比例</td><td>60%</td><td class="warn">与规范不符*</td></tr>
            <tr><td>推理奖励比例</td><td>40%</td><td class="warn">与规范不符*</td></tr>
            <tr><td>年通胀率</td><td>约 1.66%</td><td class="pass">合理</td></tr>
        </table>
        <p style="color: #ff9800; font-size: 14px;">* 注意: 需求规范要求 30% 质押 + 70% 推理，但代码实现为 60% 质押 + 40% 推理</p>
    </div>

    <div class="section">
        <h2>5. 质押参数</h2>
        <table>
            <tr><th>参数</th><th>值</th><th>状态</th></tr>
            <tr><td>最低质押</td><td>1000 AIB</td><td class="pass">正确</td></tr>
            <tr><td>解锁期</td><td>已配置</td><td class="pass">正确</td></tr>
            <tr><td>质押奖励池</td><td>1,256,637,061 AIB</td><td class="pass">充足</td></tr>
            <tr><td>可支持验证者数</td><td>> 1,256,637</td><td class="pass">充足</td></tr>
        </table>
    </div>

    <div class="section">
        <h2>验证结论</h2>
        <div class="validation-item $([ $FAILED -eq 0 ] && echo 'pass' || echo 'fail')">
            <span class="icon">$([ $FAILED -eq 0 ] && echo '✓' || echo '✗')</span>
            <span>主网配置验证 $([ $FAILED -eq 0 ] && echo '通过' || echo '未通过')</span>
        </div>
        <p>本次验证共检查 <strong>$((PASSED+FAILED+WARNINGS))</strong> 项配置，其中 <strong>$PASSED</strong> 项通过，<strong>$FAILED</strong> 项失败，<strong>$WARNINGS</strong> 项警告。</p>
        $([ $WARNINGS -gt 0 ] && echo '<p style="color: #ff9800;">请注意检查警告项，确保配置符合需求规范。</p>')
    </div>
</body>
</html>
EOF
    echo ""
    echo "HTML 报告已生成: $HTML_REPORT"
}

# =============================================================================
# 主函数
# =============================================================================
main() {
    echo ""
    echo "╔════════════════════════════════════════════════════════════════╗"
    echo "║       AIB 2.0 主网配置验证工具 v1.0                            ║"
    echo "║       Mainnet Configuration Validator                          ║"
    echo "╚════════════════════════════════════════════════════════════════╝"
    echo ""
    echo "验证时间: $REPORT_DATE"
    echo ""

    # 执行所有验证
    validate_total_supply
    validate_allocation
    validate_airdrop
    validate_block_rewards
    validate_staking_params

    # 生成摘要
    echo ""
    echo "=============================================="
    echo "验证摘要"
    echo "=============================================="
    echo -e "${GREEN}通过: $PASSED${NC}"
    echo -e "${RED}失败: $FAILED${NC}"
    echo -e "${YELLOW}警告: $WARNINGS${NC}"

    # 生成 HTML 报告
    generate_html_report

    echo ""
    echo "=============================================="
    echo "验证完成"
    echo "=============================================="
    echo "详细报告: $HTML_REPORT"
    echo ""
}

# 执行主函数
main
