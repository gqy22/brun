#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
CACHE_ROOT="${BRUN_GUIDE_CACHE:-${ROOT}/.cache/guide-data}"
INPUT="${CACHE_ROOT}/downloads/bcftools-check-1.22/check.vcf"
WORK="${CACHE_ROOT}/work/bcftools-$(date +%Y%m%d-%H%M%S)-$$"
EXPECTED_SHA256="a2253ac7dfc40829c8e8e8fc6b8a7a6635bd0cab42c17a5fc0393f4016d6c866"

check_sha256() {
  local expected="$1"
  local file="$2"
  local actual
  if command -v sha256sum >/dev/null; then
    actual="$(sha256sum "${file}" | awk '{print $1}')"
  else
    actual="$(shasum -a 256 "${file}" | awk '{print $1}')"
  fi
  [[ "${actual}" == "${expected}" ]]
}

command -v bcftools >/dev/null || {
  echo "需要 bcftools 才能运行验证" >&2
  exit 127
}

if [[ ! -f "${INPUT}" ]]; then
  echo "缺少验证数据，请先运行: make guide-data" >&2
  exit 2
fi
check_sha256 "${EXPECTED_SHA256}" "${INPUT}" || {
  echo "验证数据校验失败，请删除后重新运行: make guide-data" >&2
  exit 2
}

mkdir -p "${WORK}"
trap 'rm -rf "${WORK}"' EXIT

# 官方 check.vcf 的记录顺序与 Header 中的 contig 顺序不同；先按 Header
# 声明的参考顺序排序，生成可索引的验证输入。
bcftools sort -Oz -o "${WORK}/input.vcf.gz" "${INPUT}"
bcftools index "${WORK}/input.vcf.gz"

# 验证压缩中间文件与 -Ou 管道产生相同记录。
bcftools view -Oz -o "${WORK}/compressed-step.vcf.gz" "${WORK}/input.vcf.gz"
bcftools filter -i 'QUAL>=20' -Oz -o "${WORK}/baseline.vcf.gz" "${WORK}/compressed-step.vcf.gz"
bcftools view -Ou "${WORK}/input.vcf.gz" |
  bcftools filter -i 'QUAL>=20' -Oz -o "${WORK}/uncompressed-pipeline.vcf.gz"
bcftools view -H "${WORK}/baseline.vcf.gz" > "${WORK}/baseline.records"
bcftools view -H "${WORK}/uncompressed-pipeline.vcf.gz" > "${WORK}/pipeline.records"
diff -u "${WORK}/baseline.records" "${WORK}/pipeline.records"

# 验证按完整 contig 拆分并 concat 后记录与原始输入一致。
for contig in 1 3 4 2; do
  bcftools view -r "${contig}" -Oz -o "${WORK}/${contig}.vcf.gz" "${WORK}/input.vcf.gz"
done
bcftools concat -Oz -o "${WORK}/concatenated.vcf.gz" \
  "${WORK}/1.vcf.gz" "${WORK}/3.vcf.gz" "${WORK}/4.vcf.gz" "${WORK}/2.vcf.gz"
bcftools view -H "${WORK}/input.vcf.gz" > "${WORK}/input.records"
bcftools view -H "${WORK}/concatenated.vcf.gz" > "${WORK}/concatenated.records"
diff -u "${WORK}/input.records" "${WORK}/concatenated.records"

# 验证 regions 使用记录重叠，而 targets 默认只检查 POS。
REGION_COUNT="$(bcftools view -H -r '1:3062916-3062916' "${WORK}/input.vcf.gz" | wc -l | tr -d ' ')"
TARGET_COUNT="$(bcftools view -H -t '1:3062916-3062916' "${WORK}/input.vcf.gz" | wc -l | tr -d ' ')"
REGION_POS_COUNT="$(bcftools view -H --regions-overlap 0 -r '1:3062916-3062916' "${WORK}/input.vcf.gz" | wc -l | tr -d ' ')"
[[ "${REGION_COUNT}" == "1" && "${TARGET_COUNT}" == "0" && "${REGION_POS_COUNT}" == "0" ]] || {
  echo "regions/targets 重叠语义验证失败" >&2
  exit 1
}

# 验证 norm -m -any 将每个 ALT 拆为一条记录，并保持样本顺序。
bcftools norm -m -any -Oz -o "${WORK}/split.vcf.gz" "${WORK}/input.vcf.gz"
EXPECTED_SPLIT_RECORDS="$(bcftools query -f '%ALT\n' "${WORK}/input.vcf.gz" |
  awk -F ',' '{count += NF} END {print count+0}')"
ACTUAL_SPLIT_RECORDS="$(bcftools view -H "${WORK}/split.vcf.gz" | wc -l | tr -d ' ')"
[[ "${ACTUAL_SPLIT_RECORDS}" == "${EXPECTED_SPLIT_RECORDS}" ]] || {
  echo "多等位拆分记录数不符: ${ACTUAL_SPLIT_RECORDS} != ${EXPECTED_SPLIT_RECORDS}" >&2
  exit 1
}
if bcftools query -f '%ALT\n' "${WORK}/split.vcf.gz" | grep -q ','; then
  echo "多等位拆分后仍存在多个 ALT" >&2
  exit 1
fi
diff -u \
  <(bcftools query --list-samples "${WORK}/input.vcf.gz") \
  <(bcftools query --list-samples "${WORK}/split.vcf.gz")

# 验证 merge 用于组合不同样本；concat 会拒绝不同的样本列。
bcftools view -s A -Oz -o "${WORK}/sample-a.vcf.gz" "${WORK}/input.vcf.gz"
bcftools view -s B -Oz -o "${WORK}/sample-b.vcf.gz" "${WORK}/input.vcf.gz"
bcftools index "${WORK}/sample-a.vcf.gz"
bcftools index "${WORK}/sample-b.vcf.gz"
bcftools merge -Oz -o "${WORK}/merged-samples.vcf.gz" \
  "${WORK}/sample-a.vcf.gz" "${WORK}/sample-b.vcf.gz"
printf 'A\nB\n' > "${WORK}/expected.samples"
bcftools query --list-samples "${WORK}/merged-samples.vcf.gz" > "${WORK}/merged.samples"
diff -u "${WORK}/expected.samples" "${WORK}/merged.samples"
MERGED_RECORDS="$(bcftools view -H "${WORK}/merged-samples.vcf.gz" | wc -l | tr -d ' ')"
INPUT_RECORDS="$(bcftools view -H "${WORK}/input.vcf.gz" | wc -l | tr -d ' ')"
[[ "${MERGED_RECORDS}" == "${INPUT_RECORDS}" ]] || {
  echo "merge 前后记录数不符: ${MERGED_RECORDS} != ${INPUT_RECORDS}" >&2
  exit 1
}
if bcftools concat -Oz -o "${WORK}/invalid-concat.vcf.gz" \
  "${WORK}/sample-a.vcf.gz" "${WORK}/sample-b.vcf.gz" >/dev/null 2>&1; then
  echo "concat 不应接受不同样本列" >&2
  exit 1
fi

printf '验证通过: bcftools %s\n' "$(bcftools --version-only)"
