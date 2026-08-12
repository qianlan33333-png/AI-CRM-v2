#!/usr/bin/env ruby
require "date"
require "json"
require "open3"
require "time"
require "yaml"

def fail(message)
  warn "slice-ledger-history: #{message}"
  exit 1
end

def option(name)
  index = ARGV.index(name)
  fail("missing #{name}") unless index && ARGV[index + 1]
  ARGV[index + 1]
end

def options(name)
  ARGV.each_index.each_with_object([]) do |index, found|
    next unless ARGV[index] == name
    fail("missing value after #{name}") unless ARGV[index + 1]
    found << ARGV[index + 1]
  end
end

def canonical(value)
  case value
  when Hash
    value.keys.sort_by(&:to_s).to_h { |key| [key.to_s, canonical(value[key])] }
  when Array
    value.map { |item| canonical(item) }
  when Time
    value.utc.iso8601(6)
  when Date
    value.iso8601
  else
    value
  end
end

def normalize_legacy_flow_scalars(source)
  source.gsub(/^(\s*correction_notes:\s*)\[([^\]]*)\]$/) do
    "#{$1}[#{$2.split(",").map { |value| value.strip.to_json }.join(", ")}]"
  end
end

def read_ledger(spec)
  source, status = Open3.capture2("git", "show", spec)
  fail("cannot read #{spec}") unless status.success?
  document = YAML.safe_load(normalize_legacy_flow_scalars(source), permitted_classes: [Date, Time], aliases: false)
  fail("#{spec} must contain a slices array") unless document.is_a?(Hash) && document["slices"].is_a?(Array)

  document.fetch("slices").each_with_object({}) do |entry, entries|
    slice_id = entry.is_a?(Hash) ? entry["slice_id"] : nil
    fail("#{spec} contains an entry without slice_id") unless slice_id.is_a?(String) && !slice_id.empty?
    fail("#{spec} repeats slice_id #{slice_id}") if entries.key?(slice_id)
    entries[slice_id] = JSON.generate(canonical(entry))
  end
rescue Psych::Exception => error
  fail("#{spec} is not valid YAML: #{error.message}")
end

base = read_ledger(option("--base"))
candidate = read_ledger(option("--candidate"))
base.each do |slice_id, base_json|
  candidate_json = candidate[slice_id]
  fail("historical slice_id removed: #{slice_id}") unless candidate_json
  fail("historical slice_id drifted: #{slice_id}") unless candidate_json == base_json
end
options("--assert-unchanged").each do |slice_id|
  fail("required historical slice_id is absent from base: #{slice_id}") unless base.key?(slice_id)
  fail("required historical slice_id drifted: #{slice_id}") unless candidate[slice_id] == base[slice_id]
end
appended = candidate.keys.reject { |slice_id| base.key?(slice_id) }
puts "slice-ledger-history: PASS base=#{base.size} candidate=#{candidate.size} appended=#{appended.empty? ? "none" : appended.join(",")}"
