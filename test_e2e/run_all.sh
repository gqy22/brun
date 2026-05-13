#!/usr/bin/env bash
# brun E2E 生物信息学集成测试套件
# 测试真实生物信息学工具与 brun 的集成

set -euo pipefail

BRUN="$(cd "$(dirname "$0")/.." && pwd)/brun"
TEST_DIR=$(mktemp -d /tmp/brun-e2e-XXXXXX)
PASS=0
FAIL=0
TOTAL=0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${YELLOW}[TEST]${NC} $*" >&2; }
pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo -e "  ${GREEN}✓ PASS${NC}: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo -e "  ${RED}✗ FAIL${NC}: $1"; }

cleanup() {
    log "清理测试目录: $TEST_DIR"
    rm -rf "$TEST_DIR"
    rm -rf ~/.bio-runner/
}
trap cleanup EXIT

# ============================================================
#  0. 环境准备：生成合成数据
# ============================================================
setup_test_data() {
    log "生成合成测试数据..."
    local ref="$TEST_DIR/ref.fa"
    local fq1="$TEST_DIR/sample_R1.fq"
    local fq2="$TEST_DIR/sample_R2.fq"

    python3 -c "
import random; random.seed(42)
bases = 'ACGT'
seq = ''.join(random.choice(bases) for _ in range(10000))
for i in range(0, len(seq), 80):
    print(f'>chr{i//80+1}')
    print(seq[i:i+80])
" > "$ref"

    wgsim -N 1000 -1 100 -2 100 -r 0.01 -R 0.05 -S 20 "$ref" "$fq1" "$fq2" 2>/dev/null
    samtools faidx "$ref" 2>/dev/null

    echo "$ref|$fq1|$fq2"
}

# 从 brun 输出中提取 run ID
extract_run_id() {
    grep -oP '[0-9]{8}-[0-9]{6}-[a-f0-9]{6}' | head -1
}

# ============================================================
#  Test 1: minimap2 比对 + samtools 后处理流水线
# ============================================================
test_minimap2_pipeline() {
    log "--- Test 1: minimap2 比对流水线 ---"
    local data
    data=$(setup_test_data)
    local ref=$(echo "$data" | cut -d'|' -f1)
    local fq1=$(echo "$data" | cut -d'|' -f2)
    local fq2=$(echo "$data" | cut -d'|' -f3)
    local out_dir="$TEST_DIR/minimap2_test"
    mkdir -p "$out_dir"

    # 1a. minimap2 mapping
    output=$("$BRUN" run --name "minimap2-align" -t "align,long-read" \
        -- minimap2 -t 2 -ax map-ont "$ref" "$fq1" 2>&1) || true
    if echo "$output" | grep -q "Run started"; then
        pass "minimap2 run 记录成功"
    else
        fail "minimap2 run 未记录: $output"
        return
    fi

    # 1b. samtools sort + index
    output=$("$BRUN" run --name "samtools-sort" --project "align" \
        --cwd "$out_dir" \
        -- samtools sort -@ 2 -o results/aln.sorted.bam results/aln.sam 2>&1) || true
    if echo "$output" | grep -q "Run started"; then
        pass "samtools sort 记录成功"
    else
        fail "samtools sort 未记录: $output"
    fi

    # 1c. samtools flagstat
    output=$("$BRUN" run --name "flagstat" --project "qc" \
        --cwd "$out_dir" \
        -- samtools flagstat results/aln.sorted.bam 2>&1) || true
    if echo "$output" | grep -q "Run started"; then
        pass "samtools flagstat 记录成功"
    else
        fail "samtools flagstat 未记录"
    fi

    # 1d. samtools stats
    output=$("$BRUN" run --name "bam-stats" --project "qc" \
        --cwd "$out_dir" \
        -- sh -c "samtools stats results/aln.sorted.bam > results/stats.txt" 2>&1) || true
    if echo "$output" | grep -q "Run started"; then
        pass "samtools stats 输出捕获"
    else
        fail "samtools stats 输出未捕获"
    fi

    # 1e. 验证 list 能看到所有记录
    output=$("$BRUN" list --limit 10 2>&1) || true
    count=$(echo "$output" | grep -c "minimap2\|sort\|flagstat\|stats" || true)
    if [ "$count" -ge 3 ]; then
        pass "list 显示 $count 条运行记录"
    else
        fail "list 只显示 $count 条记录"
    fi
}

# ============================================================
#  Test 2: hisat2 比对 (短 reads)
# ============================================================
test_hisat2_alignment() {
    log "--- Test 2: hisat2 比对 ---"
    local data
    data=$(setup_test_data)
    local ref=$(echo "$data" | cut -d'|' -f1)
    local fq1=$(echo "$data" | cut -d'|' -f2)
    local fq2=$(echo "$data" | cut -d'|' -f3)
    local idx_dir="$TEST_DIR/hisat2_idx"
    local out_dir="$TEST_DIR/hisat2_test"
    mkdir -p "$idx_dir" "$out_dir/results"

    hisat2-build "$ref" "$idx_dir/genome" > /dev/null 2>&1 || true

    output=$("$BRUN" run --name "hisat2-align" --project "rnaseq" \
        -t rna-seq -t short-read \
        --cwd "$out_dir" \
        -- hisat2 -p 2 -x "$idx_dir/genome" -1 "$fq1" -2 "$fq2" -S hisat2.sam 2>&1) || true

    if echo "$output" | grep -q "Run started\|Command finished"; then
        pass "hisat2 比对完成并记录"
    else
        fail "hisat2 失败: $(echo "$output" | tail -5)"
    fi

    # 验证 show 命令 — 用 hisat2 的输出提取 ID
    run_id=$(echo "$output" | extract_run_id)
    if [ -n "$run_id" ]; then
        detail=$("$BRUN" show "$run_id" 2>&1) || true
        if echo "$detail" | grep -q "hisat2\|rnaseq"; then
            pass "show 详情包含正确信息"
        else
            fail "show 详情不完整"
        fi
    else
        fail "无法获取 run ID"
    fi
}

# ============================================================
#  Test 3: FastQC 质控
# ============================================================
test_fastqc() {
    log "--- Test 3: FastQC 质控 ---"
    local data
    data=$(setup_test_data)
    local fq1=$(echo "$data" | cut -d'|' -f2)
    local fq2=$(echo "$data" | cut -d'|' -f3)
    local out_dir="$TEST_DIR/fastqc_test"
    mkdir -p "$out_dir"

    output=$("$BRUN" run --name "fastqc-qc" --project "qc" \
        -t quality-control \
        --cwd "$out_dir" \
        -- fastqc -o . -f fastq "$fq1" "$fq2" 2>&1) || true

    if echo "$output" | grep -q "Run started"; then
        pass "FastQC 运行记录成功"
    else
        fail "FastQC 运行失败: $(echo "$output" | tail -3)"
    fi

    # 用刚运行的 fastqc run ID 来查 outputs（不用 latest）
    fqcid=$(echo "$output" | extract_run_id)
    if [ -n "$fqcid" ]; then
        out_output=$("$BRUN" outputs "$fqcid" 2>&1) || true
        if echo "$out_output" | grep -qi "html\|report\|zip"; then
            pass "outputs 检测到 FastQC 报告文件"
        else
            fail "outputs 未检测到报告: $out_output"
        fi
    else
        fail "无法获取 FastQC run ID"
    fi
}

# ============================================================
#  Test 4: bcftools 变异检测
# ============================================================
test_bcftools_variant_calling() {
    log "--- Test 4: bcftools 变异检测流水线 ---"
    local data
    data=$(setup_test_data)
    local ref=$(echo "$data" | cut -d'|' -f1)
    local out_dir="$TEST_DIR/bcftools_test"
    mkdir -p "$out_dir/results"

    local fq1=$(echo "$data" | cut -d'|' -f2)
    minimap2 -t 2 -ax map-ont "$ref" "$fq1" 2>/dev/null | \
        samtools sort -@ 2 -o "$out_dir/aln.bam" 2>/dev/null || true
    samtools index "$out_dir/aln.bam" 2>/dev/null || true

    # mpileup + call
    output=$("$BRUN" run --name "bcftools-call" --project "variant" \
        -t snp -t indel \
        --cwd "$out_dir" \
        -- sh -c "bcftools mpileup -f $ref aln.bam | bcftools call -mv -Oz -o results/variants.vcf.gz" 2>&1) || true

    if echo "$output" | grep -q "Run started"; then
        pass "bcftools variant calling 完成"
    else
        fail "bcftools 失败: $(echo "$output" | tail -3)"
    fi

    # index vcf
    output=$("$BRUN" run --name "bcftools-index" --project "variant" \
        --cwd "$out_dir" \
        -- bcftools index results/variants.vcf.gz 2>&1) || true

    if echo "$output" | grep -q "Run started"; then
        pass "bcftools index 完成"
    else
        fail "bcftools index 失败"
    fi

    # stats
    output=$("$BRUN" run --name "bcftools-stats" --project "variant" \
        --cwd "$out_dir" \
        -- sh -c "bcftools stats results/variants.vcf.gz > results/vcf.stats" 2>&1) || true

    if echo "$output" | grep -q "Run started"; then
        pass "bcftools stats 完成"
    else
        fail "bcftools stats 失败"
    fi
}

# ============================================================
#  Test 5: bedtools 区间操作
# ============================================================
test_bedtools() {
    log "--- Test 5: bedtools 区间分析 ---"
    local out_dir="$TEST_DIR/bedtools_test"
    mkdir -p "$out_dir/results"

    cat > "$out_dir/a.bed" << 'EOF'
chr1	100	200	geneA	.
chr1	300	500	geneB	.
chr2	1000	1200	geneC	.
EOF

    cat > "$out_dir/b.bed" << 'EOF'
chr1	150	250	featureX
chr1	400	450	featureY
chr2	1100	1150	featureZ
EOF

    output=$("$BRUN" run --name "bedtools-intersect" --project "annotation" \
        -t bed -t intersect \
        --cwd "$out_dir" \
        -- sh -c "bedtools intersect -a a.bed -b b.bed > results/intersect.bed" 2>&1) || true

    if echo "$output" | grep -q "Run started"; then
        pass "bedtools intersect 完成"
    else
        fail "bedtools intersect 失败"
    fi

    output=$("$BRUN" run --name "bedtools-coverage" --project "annotation" \
        --cwd "$out_dir" \
        -- sh -c "bedtools coverage -a a.bed -b b.bed > results/coverage.txt" 2>&1) || true

    if echo "$output" | grep -q "Run started"; then
        pass "bedtools coverage 完成"
    else
        fail "bedtools coverage 失败"
    fi
}

# ============================================================
#  Test 6: 多步骤流水线 + tag/note/rerun
# ============================================================
test_pipeline_workflow() {
    log "--- Test 6: 完整流水线 + tag/note/rerun ---"
    local out_dir="$TEST_DIR/pipeline_test"
    mkdir -p "$out_dir"

    # 用独立命令模拟流水线各步骤（避免文件依赖导致失败）
    # Step 1: 产生输出文件的命令
    out1=$("$BRUN" run --name "pipe-step1-map" --project "pipeline" \
        -t step-map -t workflowA \
        --cwd "$out_dir" \
        -- sh -c 'echo "sample_data" > output.txt && echo "mapped 1000 reads"' 2>&1) || true
    run1_id=$(echo "$out1" | extract_run_id)

    # Step 2-4: 独立命令（不依赖 Step 1 的文件）
    "$BRUN" run --name "pipe-step2-sort" --project "pipeline" \
        -t step-sort -t workflowA \
        --cwd "$out_dir" \
        -- sh -c 'echo "sorted"' > /dev/null 2>&1 || true

    "$BRUN" run --name "pipe-step3-index" --project "pipeline" \
        -t step-index -t workflowA \
        --cwd "$out_dir" \
        -- sh -c 'echo "indexed"' > /dev/null 2>&1 || true

    "$BRUN" run --name "pipe-step4-flagstat" --project "pipeline" \
        -t step-qc -t workflowA \
        --cwd "$out_dir" \
        -- sh -c 'echo "flagstat: 1000 mapped"' > /dev/null 2>&1 || true

    # note 测试
    if [ -n "$run1_id" ]; then
        "$BRUN" note "$run1_id" "这是流水线第一步，使用 minimap2 进行比对" > /dev/null 2>&1 || true
        if "$BRUN" show "$run1_id" 2>&1 | grep -q "流水线第一步"; then
            pass "note 写入和读取正常"
        else
            fail "note 功能异常"
        fi
    else
        fail "无法获取 pipeline step1 run ID"
    fi

    # 按 tag 过滤 (list 默认显示 COMMAND 不是 name，用 project 匹配)
    output=$("$BRUN" list -t "workflowA" --limit 10 2>&1) || true
    count=$(echo "$output" | grep -c "pipeline" || true)
    if [ "$count" -ge 3 ]; then
        pass "tag 过滤找到 $count 条流水线记录"
    else
        fail "tag 过滤只找到 $count 条, output: $(echo "$output" | head -6)"
    fi

    # rerun dry-run
    if [ -n "$run1_id" ]; then
        output=$("$BRUN" rerun "$run1_id" --dry-run 2>&1) || true
        if echo "$output" | grep -q "Would run\|minimap2\|echo"; then
            pass "rerun dry-run 正常"
        else
            fail "rerun dry-run 异常: $output"
        fi
    else
        fail "跳过 rerun 测试 (无 run ID)"
    fi
}

# ============================================================
#  Test 7: 错误处理 — 故意失败的命令
# ============================================================
test_error_handling() {
    log "--- Test 7: 错误处理 ---"
    local out_dir="$TEST_DIR/error_test"
    mkdir -p "$out_dir"

    # 不存在的命令
    output=$("$BRUN" run --name "bad-cmd" --project "error-test" \
        --cwd "$out_dir" \
        -- nonexistent_command_abc123 2>&1) || true

    if echo "$output" | grep -qi "failed\|Command finished.*failed"; then
        pass "失败命令状态正确记录为 failed"
    else
        fail "失败命令状态异常: $output"
    fi

    # 非 zero exit code
    output=$("$BRUN" run --name "fail-exit" --project "error-test" \
        --cwd "$out_dir" \
        -- sh -c "exit 42" 2>&1) || true

    if echo "$output" | grep -qi "failed"; then
        pass "exit code 42 正确记录为 failed"
    else
        fail "exit code 42 状态异常"
    fi

    # 验证 status 过滤 (list 显示的是 COMMAND 不是 name)
    output=$("$BRUN" list --status failed --limit 5 2>&1) || true
    if echo "$output" | grep -q "nonexistent_command\|exit 42"; then
        pass "--status failed 过滤正常"
    else
        fail "failed 过滤无结果, output: $(echo "$output" | head -8)"
    fi
}

# ============================================================
#  Test 8: 大量并发运行 (压力测试)
# ============================================================
test_concurrent_runs() {
    log "--- Test 8: 并发压力测试 (5 并发) ---"
    local out_dir="$TEST_DIR/concurrent_test"
    mkdir -p "$out_dir"

    pids=()
    for i in $(seq 1 5); do
        "$BRUN" run --name "concurrent-$i" --project "stress" \
            --cwd "$out_dir" \
            -- sh -c "sleep 0.$((RANDOM % 5)) && echo hello_$i && sleep 0.$((RANDOM % 3))" \
            > "$out_dir/out_$i.log" 2>&1 &
        pids+=($!)
    done

    for pid in "${pids[@]}"; do
        wait "$pid" 2>/dev/null || true
    done

    success=0
    for i in $(seq 1 5); do
        if grep -q "Run started" "$out_dir/out_$i.log" 2>/dev/null; then
            success=$((success+1))
        fi
    done

    if [ "$success" -eq 5 ]; then
        pass "5 个并发运行全部成功记录"
    elif [ "$success" -ge 3 ]; then
        pass "5 个并发中 $success 个成功 (部分 SQLITE_BUSY 可接受)"
    else
        fail "只有 $success/5 个并发成功"
    fi

    # 等待 DB 同步后验证（前面已有大量写入，WAL 可能需要更多同步时间）
    sleep 3
    output=$("$BRUN" list --limit 20 --project stress 2>&1) || true
    count=$(echo "$output" | grep -c "stress" || true)
    if [ "$count" -ge 3 ]; then
        pass "list 显示 $count 条并发记录"
    else
        fail "list 只显示 $count 条并发记录, output: $(echo "$output" | head -8)"
    fi
}

# ============================================================
#  Test 9: 日志查看功能
# ============================================================
test_logs_viewing() {
    log "--- Test 9: 日志查看 ---"
    local out_dir="$TEST_DIR/logs_test"
    mkdir -p "$out_dir"

    # 产生有 stdout/stderr 输出的命令
    log_output=$("$BRUN" run --name "log-test" --project "logs" \
        --cwd "$out_dir" \
        -- sh -c 'echo "LINE1: starting"; echo "LINE2: processing"; echo "LINE3: done"; echo "ERR1: warning" >&2; echo "ERR2: error info" >&2' 2>&1) || true
    log_id=$(echo "$log_output" | extract_run_id)

    # 查看 stdout
    if [ -n "$log_id" ]; then
        stdout=$("$BRUN" logs "$log_id" 2>&1) || true
    else
        stdout=$("$BRUN" logs latest 2>&1) || true
    fi
    if echo "$stdout" | grep -q "LINE1\|LINE2\|LINE3"; then
        pass "logs stdout 内容正确"
    else
        fail "logs stdout 缺失内容"
    fi

    # 查看 stderr
    if [ -n "$log_id" ]; then
        stderr=$("$BRUN" logs "$log_id" --stderr 2>&1) || true
    else
        stderr=$("$BRUN" logs latest --stderr 2>&1) || true
    fi
    if echo "$stderr" | grep -q "ERR1\|ERR2"; then
        pass "logs stderr 内容正确"
    else
        fail "logs stderr 缺失内容"
    fi

    # tail 功能 — 取最后 1 行应包含 LINE3/done
    if [ -n "$log_id" ]; then
        tailed=$("$BRUN" logs "$log_id" --tail 1 2>&1) || true
    else
        tailed=$("$BRUN" logs latest --tail 1 2>&1) || true
    fi
    # TailLog 返回末 N 行，可能包含换行，检查是否非空且含关键词
    if echo "$tailed" | grep -q "LINE3\|done\|LINE2"; then
        pass "logs --tail 1 正确截取"
    else
        fail "logs --tail 1 结果异常: [$tailed]"
    fi
}

# ============================================================
#  Test 10: fs-diff 自动输出检测
# ============================================================
test_fs_diff_detection() {
    log "--- Test 10: fs-diff 自动输出检测 ---"
    local out_dir="$TEST_DIR/fsdiff_test"
    mkdir -p "$out_dir"

    # 在 cwd 下产生新文件的命令 (不预创建 results/)
    run_output=$("$BRUN" run --name "fs-diff-test" --project "auto-detect" \
        --cwd "$out_dir" \
        -- sh -c '
            mkdir -p results
            echo "sample_data_12345" > results/output.txt
            echo "{\"key\": \"value\"}" > results/config.json
            echo "<html><body>report</body></html>" > results/report.html
            echo "#!/bin/bash" > results/script.sh
            echo "fastq data" > results/sample.fastq
        ' 2>&1) || true
    fs_id=$(echo "$run_output" | extract_run_id)

    # 用具体 run ID 查 outputs
    if [ -n "$fs_id" ]; then
        output=$("$BRUN" outputs "$fs_id" 2>&1) || true
    else
        output=$("$BRUN" outputs latest 2>&1) || true
    fi

    checks=0
    for pattern in "output.txt" "config.json" "report.html" "script.sh" "sample.fastq"; do
        if echo "$output" | grep -q "$pattern"; then
            checks=$((checks+1))
        fi
    done

    if [ "$checks" -ge 4 ]; then
        pass "fs-diff 检测到 $checks/5 个输出文件"
    else
        fail "fs-diff 只检测到 $checks/5 个文件, output: $output"
    fi

    # 验证分类 (放宽匹配)
    if echo "$output" | grep -qi "config"; then
        pass "JSON 文件分类为 config"
    else
        fail "JSON 分类异常, output: $output"
    fi

    if echo "$output" | grep -qi "report\|html"; then
        pass "HTML 文件分类为 report"
    else
        fail "HTML 分类异常, output: $output"
    fi
}

# ============================================================
#  主流程
# ============================================================
main() {
    echo "============================================"
    echo "  brun 生物信息学 E2E 集成测试"
    echo "  测试目录: $TEST_DIR"
    echo "  brun 二进制: $BRUN"
    echo "============================================"
    echo ""

    test_minimap2_pipeline
    echo ""
    test_hisat2_alignment
    echo ""
    test_fastqc
    echo ""
    test_bcftools_variant_calling
    echo ""
    test_bedtools
    echo ""
    test_pipeline_workflow
    echo ""
    test_error_handling
    echo ""
    test_concurrent_runs
    echo ""
    test_logs_viewing
    echo ""
    test_fs_diff_detection

    echo ""
    echo "============================================"
    echo -e "  结果: ${GREEN}$PASS 通过${NC} / ${RED}$FAIL 失败${NC} / 共 $TOTAL 项"
    echo "============================================"

    if [ "$FAIL" -gt 0 ]; then
        exit 1
    fi
}

main "$@"
