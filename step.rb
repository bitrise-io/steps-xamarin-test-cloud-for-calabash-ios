require 'optparse'
require_relative 'utils/logger'

# -----------------------
# --- functions
# -----------------------

def to_bool(value)
  return true if value == true || value =~ (/^(true|t|yes|y|1)$/i)
  return false if value == false || value.nil? || value =~ (/^(false|f|no|n|0)$/i)
  fail_with_message("Invalid value for Boolean: \"#{value}\"")
end

# -----------------------
# --- main
# -----------------------

#
# Input validation
options = {
  ipa_path: nil,
  dsym_path: nil,
  api_key: nil,
  user: nil,
  devices: nil,
  async: true,
  series: 'master',
  other_parameters: nil
}

parser = OptionParser.new do|opts|
  opts.banner = 'Usage: step.rb [options]'
  opts.on('-c', '--api key', 'API key') { |c| options[:api_key] = c unless c.to_s == '' }
  opts.on('-b', '--user user', 'User') { |b| options[:user] = b unless b.to_s == '' }
  opts.on('-d', '--devices devices', 'Devices') { |d| options[:devices] = d unless d.to_s == '' }
  opts.on('-e', '--async async', 'Async') { |e| options[:async] = false if to_bool(e) == false }
  opts.on('-f', '--series series', 'Series') { |f| options[:series] = f unless f.to_s == '' }
  opts.on('-g', '--other parameters', 'Other') { |g| options[:other_parameters] = g unless g.to_s == '' }
  opts.on('-i', '--ipa path', 'IPA') { |i| options[:ipa_path] = i unless i.to_s == '' }
  opts.on('-j', '--dsym path', 'DSYM') { |j| options[:dsym_path] = j unless j.to_s == '' }
  opts.on('-h', '--help', 'Displays Help') do
    exit
  end
end
parser.parse!

fail_with_message('No ipa found') unless options[:ipa_path] && File.exist?(options[:ipa_path])
fail_with_message('api_key not specified') unless options[:api_key]
fail_with_message('user not specified') unless options[:user]
fail_with_message('devices not specified') unless options[:devices]

#
# Print configs
puts
puts '========== Configs =========='
puts " * ipa_path: #{options[:ipa_path]}"
puts " * dsym_path: #{options[:dsym_path]}"
puts ' * api_key: ***'
puts " * user: #{options[:user]}"
puts " * devices: #{options[:devices]}"
puts " * async: #{options[:async]}"
puts " * series: #{options[:series]}"
puts " * other_parameters: #{options[:other_parameters]}"

# Check if there is a Gemfile in the directory
gemfile_detected = File.exist? 'Gemfile'

calabash_cucumber_gem_detected = false
xamarin_test_cloud_gem_detected = false

if gemfile_detected
  File.open('Gemfile', 'r') do |f|
    f.each_line do |line|
      calabash_cucumber_gem_detected = true if line.downcase.include? 'calabash-cucumber'
      xamarin_test_cloud_gem_detected = true if line.downcase.include? 'xamarin-test-cloud'
    end
  end
end

if calabash_cucumber_gem_detected && xamarin_test_cloud_gem_detected
  puts 'bundle install'
  system('bundle install')
else
  puts 'Gemfile found, but no calabash-cucumber and/or xamarin-test-cloud gem specified' if gemfile_detected
  puts 'Installing missings gems'

  puts 'gem install calabash-cucumber'
  system('gem install calabash-cucumber')

  puts 'gem install xamarin-test-cloud'
  system('gem install xamarin-test-cloud')
end

#
# Build Request
test_cloud_cmd = []
test_cloud_cmd << 'bundle exec' if gemfile_detected
test_cloud_cmd << "test-cloud submit \"#{options[:ipa_path]}\""
test_cloud_cmd << options[:api_key]
test_cloud_cmd << "--user=#{options[:user]}"
test_cloud_cmd << "--devices=#{options[:devices]}"
test_cloud_cmd << '--async' if options[:async]
test_cloud_cmd << "--series=#{options[:series]}" if options[:series]
test_cloud_cmd << "--dsym-file #{options[:dsym_path]}" if options[:dsym_path]
test_cloud_cmd << options[:other_parameters] if options[:other_parameters]

test_cloud_cmd_copy = test_cloud_cmd.dup
test_cloud_cmd_copy[gemfile_detected ? 2 : 1] = "***"

puts
puts test_cloud_cmd_copy.join(" ")
system(test_cloud_cmd.join(" "))
fail_with_message('test-cloud -- failed') unless $?.success?

puts
puts '(i) The result is: succeeded'
system('envman add --key BITRISE_XAMARIN_TEST_RESULT --value succeeded')
