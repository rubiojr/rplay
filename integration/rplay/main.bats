@test "rplay prints help" {
  run ./rplay
  [ "$status" -eq 0 ]
  [[ "$output" =~ "USAGE:" ]]
}
