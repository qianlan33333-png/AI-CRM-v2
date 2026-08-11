#!/usr/bin/env perl
use strict;
use warnings;

use Digest::SHA ();
use IO::Handle ();
use IPC::Open2 qw(open2);

sub fail {
  print STDERR "repo-contract: $_[0]\n";
  exit 1;
}

my $operation = shift @ARGV // '';
my @checks;
my @file_paths;
my %requested;

if ($operation eq 'required') {
  @ARGV or fail("required verifier needs at least one repository path");
  for my $file_path (@ARGV) {
    push @checks, [undef, $file_path];
  }
} elsif ($operation eq 'modes' || $operation eq 'receipts') {
  @ARGV && @ARGV % 2 == 0
    or fail("$operation verifier requires expected value and repository path pairs");
  while (@ARGV) {
    my $expected = shift @ARGV;
    my $file_path = shift @ARGV;
    if ($operation eq 'modes') {
      $expected =~ /\A(?:100644|100755)\z/
        or fail("invalid pinned repository mode");
    } else {
      $expected =~ /\A[0-9a-f]{64}\z/
        or fail("invalid pinned SHA256 for repository path");
    }
    push @checks, [$expected, $file_path];
  }
} else {
  fail("repository verifier operation must be required, modes, or receipts");
}

for my $check (@checks) {
  my (undef, $file_path) = @{$check};
  length($file_path) && $file_path !~ /[\0\r\n]/
    or fail("invalid pinned repository path");
  push @file_paths, $file_path unless $requested{$file_path}++;
}

my %index_entries;
open my $index_fh, '-|', 'git', 'ls-files', '-s', '-z', '--', @file_paths
  or fail("could not read the staged repository index");
{
  local $/ = "\0";
  while (my $record = <$index_fh>) {
    substr($record, -1, 1, '') if substr($record, -1, 1) eq "\0";
    $record =~ /\A([0-9]{6}) ([0-9a-f]{40}|[0-9a-f]{64}) ([0-3])\t(.*)\z/s
      or fail("repository path has an invalid index entry");
    my ($mode, $object_id, $stage, $file_path) = ($1, $2, $3, $4);
    push @{$index_entries{$file_path}}, [$mode, $object_id, $stage];
  }
}
close $index_fh
  or fail("could not finish reading the staged repository index");

for my $check (@checks) {
  my ($expected, $file_path) = @{$check};
  my $entries = $index_entries{$file_path} // [];
  if ($operation eq 'required') {
    @{$entries} == 1 && $entries->[0][2] == 0
      or fail("required file is missing or has an ambiguous index entry: $file_path");
    my $mode = $entries->[0][0];
    $mode eq '100644' || $mode eq '100755'
      or fail("required file_path must be a regular tracked file: $file_path (mode $mode)");
  } elsif ($operation eq 'modes') {
    my $actual = @{$entries} == 1 && $entries->[0][2] == 0
      ? $entries->[0][0]
      : '';
    $actual eq $expected
      or fail("pinned repository mode drifted: $file_path ($actual)");
  } else {
    @{$entries} == 1 && $entries->[0][2] == 0
      or fail("pinned repository path is missing or ambiguous in the staged index: $file_path");
  }
}

exit 0 unless $operation eq 'receipts';

my ($object_fh, $command_fh);
my $cat_file_pid = open2($object_fh, $command_fh, 'git', 'cat-file', '--batch');
binmode $object_fh;
binmode $command_fh;
$command_fh->autoflush(1);

for my $receipt (@checks) {
  my ($expected, $file_path) = @{$receipt};
  my $object_id = $index_entries{$file_path}[0][1];
  print {$command_fh} "$object_id\n"
    or fail("could not request staged content for: $file_path");

  my $header = <$object_fh>;
  defined $header && $header =~ /\A[0-9a-f]{40,64} blob ([0-9]+)\n\z/
    or fail("could not read staged content metadata for: $file_path");
  my $remaining = 0 + $1;
  my $digest = Digest::SHA->new(256);

  while ($remaining > 0) {
    my $chunk_size = $remaining > 65536 ? 65536 : $remaining;
    my $read_count = read($object_fh, my $chunk, $chunk_size);
    defined $read_count && $read_count > 0
      or fail("staged content ended early for: $file_path");
    $digest->add($chunk);
    $remaining -= $read_count;
  }

  my $separator_count = read($object_fh, my $separator, 1);
  defined $separator_count && $separator_count == 1 && $separator eq "\n"
    or fail("staged content framing is invalid for: $file_path");

  my $actual = $digest->hexdigest;
  $actual eq $expected
    or fail("pinned repository content drifted: $file_path ($actual)");
}

close $command_fh
  or fail("could not close the staged content verifier");
waitpid($cat_file_pid, 0);
$? == 0
  or fail("staged content verifier exited unsuccessfully");
