FROM golang:1.17.7

# The purpose of this Dockerfile is just to "trick" Dependabot into
# creating a pull request when a new version of Go is released. This
# way we can create a release and build it with the new Go version.
