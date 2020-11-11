@test "rplay random needs an index" {
  run ./rplay --index-path "foo.idx" random
  [ "$status" -eq 0 ]
  [[ "$output" =~ "rplay index does not exist. Use 'rplay index' to create it first" ]]
}
