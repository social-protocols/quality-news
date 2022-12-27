package main

import "embed"

//go:embed templates/*
//go:embed sql/*
//go:embed data/*
//go:embed seed/*
var resources embed.FS
