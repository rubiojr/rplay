@test "rplay random needs a repository" {
  unset RESTIC_REPOSITORY
  run ./rplay random
  [ "$status" -eq 0 ]
  [[ "$output" =~ "Fatal: Please specify repository location" ]]
}
