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

printf '验证通过: bcftools %s\n' "$(bcftools --version-only)"
