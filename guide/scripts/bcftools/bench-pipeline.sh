#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
CACHE_ROOT="${BRUN_GUIDE_CACHE:-${ROOT}/.cache/guide-data}"
TIER="smoke"
REPEATS=3
WARMUPS=1
THREADS=1

usage() {
  cat <<'EOF'
用法: bench-pipeline.sh [选项]

选项:
  --tier smoke|medium  数据级别（默认 smoke）
  --repeats N          正式重复次数（默认 3）
  --warmups N          预热次数（默认 1）
  --threads N          最终压缩线程数（默认 1）
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tier) TIER="$2"; shift 2 ;;
    --repeats) REPEATS="$2"; shift 2 ;;
    --warmups) WARMUPS="$2"; shift 2 ;;
    --threads) THREADS="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "${TIER}" in
  smoke)
    DATASET="igsr-grch38-chrx"
    FILENAME="ALL.chrX.shapeit2_integrated_snvindels_v2a_27022019.GRCh38.phased.vcf.gz"
    ;;
  medium)
    DATASET="igsr-grch38-wgs-sites"
    FILENAME="ALL.wgs.shapeit2_integrated_snvindels_v2a.GRCh38.27022019.sites.vcf.gz"
    ;;
  *) echo "--tier 只支持 smoke 或 medium" >&2; exit 2 ;;
esac

[[ "${REPEATS}" =~ ^[1-9][0-9]*$ ]] || { echo "--repeats 必须是正整数" >&2; exit 2; }
[[ "${WARMUPS}" =~ ^[0-9]+$ ]] || { echo "--warmups 必须是非负整数" >&2; exit 2; }
[[ "${THREADS}" =~ ^[1-9][0-9]*$ ]] || { echo "--threads 必须是正整数" >&2; exit 2; }
command -v bcftools >/dev/null || { echo "需要 bcftools" >&2; exit 127; }
[[ -x /usr/bin/time ]] || { echo "需要支持 -o/-f 的 /usr/bin/time" >&2; exit 127; }

INPUT="${CACHE_ROOT}/downloads/${DATASET}/${FILENAME}"
[[ -f "${INPUT}" ]] || {
  echo "缺少 ${TIER} 数据，请先运行: make guide-data TIER=${TIER}" >&2
  exit 2
}

RUN_ID="$(date +%Y%m%d-%H%M%S)-$$"
RESULT_DIR="${CACHE_ROOT}/benchmarks/bcftools-pipeline/${TIER}/${RUN_ID}"
WORK="${CACHE_ROOT}/work/bcftools-benchmark-${RUN_ID}"
mkdir -p "${RESULT_DIR}" "${WORK}"
trap 'rm -rf "${WORK}"' EXIT

INPUT_BYTES="$(wc -c < "${INPUT}" | tr -d ' ')"
RECORDS="$(bcftools index --nrecords "${INPUT}")"
SAMPLES="$(bcftools query --list-samples "${INPUT}" | awk 'END {print NR}')"
CONTIGS="$(bcftools view --header-only "${INPUT}" | awk '/^##contig=/{n++} END {print n+0}')"

{
  printf 'field\tvalue\n'
  printf 'date\t%s\n' "$(date --iso-8601=seconds 2>/dev/null || date '+%Y-%m-%dT%H:%M:%S%z')"
  printf 'dataset\t%s\n' "${DATASET}"
  printf 'tier\t%s\n' "${TIER}"
  printf 'input\t%s\n' "${INPUT}"
  printf 'input_bytes\t%s\n' "${INPUT_BYTES}"
  printf 'records\t%s\n' "${RECORDS}"
  printf 'samples\t%s\n' "${SAMPLES}"
  printf 'contigs\t%s\n' "${CONTIGS}"
  printf 'bcftools\t%s\n' "$(bcftools --version-only)"
  printf 'threads\t%s\n' "${THREADS}"
  printf 'repeats\t%s\n' "${REPEATS}"
  printf 'warmups\t%s\n' "${WARMUPS}"
  printf 'system\t%s\n' "$(uname -srm)"
} > "${RESULT_DIR}/environment.tsv"

printf 'variant\trepeat\twall_seconds\tuser_seconds\tsystem_seconds\tmax_rss_kb\n' > "${RESULT_DIR}/runs.tsv"

run_variant() {
  local variant="$1"
  local repeat="$2"
  local record="${3:-yes}"
  local metrics="${WORK}/${variant}.${repeat}.time"
  local intermediate="${WORK}/intermediate.vcf.gz"
  local output="${WORK}/${variant}.vcf.gz"
  rm -f "${intermediate}" "${output}"

  if [[ "${variant}" == "compressed-intermediate" ]]; then
    /usr/bin/time -o "${metrics}" -f '%e\t%U\t%S\t%M' \
      bash -o pipefail -c 'bcftools view -i '\''TYPE="snp"'\'' -Oz --threads "$1" -o "$2" "$3" && bcftools view -Oz --threads "$1" -o "$4" "$2"' \
      _ "${THREADS}" "${intermediate}" "${INPUT}" "${output}"
  else
    /usr/bin/time -o "${metrics}" -f '%e\t%U\t%S\t%M' \
      bash -o pipefail -c 'bcftools view -i '\''TYPE="snp"'\'' -Ou "$2" | bcftools view -Oz --threads "$1" -o "$3"' \
      _ "${THREADS}" "${INPUT}" "${output}"
  fi

  if [[ "${record}" == "yes" ]]; then
    printf '%s\t%s\t%s\n' "${variant}" "${repeat}" "$(cat "${metrics}")" >> "${RESULT_DIR}/runs.tsv"
  fi
}

echo "基准数据: ${DATASET} ($(numfmt --to=iec-i --suffix=B "${INPUT_BYTES}" 2>/dev/null || printf '%s bytes' "${INPUT_BYTES}"))"
for ((i=1; i<=WARMUPS; i++)); do
  echo "预热 ${i}/${WARMUPS}: compressed-intermediate"
  run_variant compressed-intermediate "warmup-${i}" no
  echo "预热 ${i}/${WARMUPS}: uncompressed-bcf"
  run_variant uncompressed-bcf "warmup-${i}" no
done

for ((i=1; i<=REPEATS; i++)); do
  echo "运行 ${i}/${REPEATS}: compressed-intermediate"
  run_variant compressed-intermediate "${i}"
  echo "运行 ${i}/${REPEATS}: uncompressed-bcf"
  run_variant uncompressed-bcf "${i}"
done

if command -v sha256sum >/dev/null; then
  HASH_COMMAND=(sha256sum)
else
  HASH_COMMAND=(shasum -a 256)
fi
bcftools view -H "${WORK}/compressed-intermediate.vcf.gz" | "${HASH_COMMAND[@]}" | awk '{print $1}' > "${WORK}/baseline.sha256"
bcftools view -H "${WORK}/uncompressed-bcf.vcf.gz" | "${HASH_COMMAND[@]}" | awk '{print $1}' > "${WORK}/optimized.sha256"
diff -u "${WORK}/baseline.sha256" "${WORK}/optimized.sha256"
cp "${WORK}/baseline.sha256" "${RESULT_DIR}/records.sha256"

awk -F '\t' '
  NR > 1 { count[$1]++; wall[$1]+=$3; rss[$1]+=$6 }
  END {
    print "variant\truns\tmean_wall_seconds\tmean_max_rss_kb"
    for (variant in count) {
      printf "%s\t%d\t%.3f\t%.0f\n", variant, count[variant], wall[variant]/count[variant], rss[variant]/count[variant]
    }
  }
' "${RESULT_DIR}/runs.tsv" > "${RESULT_DIR}/summary.unsorted.tsv"
{
  head -n 1 "${RESULT_DIR}/summary.unsorted.tsv"
  tail -n +2 "${RESULT_DIR}/summary.unsorted.tsv" | sort
} > "${RESULT_DIR}/summary.tsv"
rm -f "${RESULT_DIR}/summary.unsorted.tsv"

echo "结果一致，基准完成: ${RESULT_DIR}"
cat "${RESULT_DIR}/summary.tsv"
