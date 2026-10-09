package atmosphere

import "math"

// A periodic volume with a true 3D mip chain, packed into a 2D slice atlas.
// Each level has its own one-voxel gutters. The engine exposes only 2D images;
// hardware 2D mip generation would blend unrelated slices/levels together.
const cloudNoiseSize = 64
const cloudNoiseTile = cloudNoiseSize + 2
const cloudNoiseWidth = cloudNoiseTile * 8
const cloudNoiseHeight = 723 // rows: 528 + 136 + 36 + 10 + 6 + 4 + 3

func cloudHash(x, y, z int, seed uint64) uint32 {
	h := uint32(x)*1597334677 ^ uint32(y)*3812015801 ^ uint32(z)*2798796415 ^ uint32(seed) ^ uint32(seed>>32)
	h = (h ^ (h >> 16)) * 2246822519
	h = (h ^ (h >> 13)) * 3266489917
	return h ^ (h >> 16)
}

func cloudValue(x, y, z float64, period int, seed uint64) float64 {
	xi, yi, zi := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
	fade := func(t float64) float64 { return t * t * t * (t*(t*6-15) + 10) }
	fx, fy, fz := fade(x-float64(xi)), fade(y-float64(yi)), fade(z-float64(zi))
	weight := func(bit int, f float64) float64 {
		if bit == 0 {
			return 1 - f
		}
		return f
	}
	var sum float64
	for dz := 0; dz < 2; dz++ {
		for dy := 0; dy < 2; dy++ {
			for dx := 0; dx < 2; dx++ {
				h := cloudHash((xi+dx)%period, (yi+dy)%period, (zi+dz)%period, seed)
				sum += float64(h) / 4294967295 * weight(dx, fx) * weight(dy, fy) * weight(dz, fz)
			}
		}
	}
	return sum
}

func cloudCell(x, y, z float64, period int, seed uint64) float64 {
	xi, yi, zi := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
	nearest := 4.0
	for dz := -1; dz <= 1; dz++ {
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				cx, cy, cz := xi+dx, yi+dy, zi+dz
				h := cloudHash((cx+period)%period, (cy+period)%period, (cz+period)%period, seed)
				a := float64(cx) + float64(h&1023)/1024 - x
				b := float64(cy) + float64((h>>10)&1023)/1024 - y
				c := float64(cz) + float64((h>>20)&1023)/1024 - z
				nearest = math.Min(nearest, a*a+b*b+c*c)
			}
		}
	}
	return 1 - math.Min(math.Sqrt(nearest), 1)
}

func cloudNoiseAtlas(seed uint64) []byte {
	volume := make([]byte, cloudNoiseSize*cloudNoiseSize*cloudNoiseSize*4)
	pack := func(v float64) byte { return byte(math.Round(math.Max(0, math.Min(1, v)) * 255)) }
	for z := 0; z < cloudNoiseSize; z++ {
		for y := 0; y < cloudNoiseSize; y++ {
			for x := 0; x < cloudNoiseSize; x++ {
				p := [3]float64{float64(x) / cloudNoiseSize, float64(y) / cloudNoiseSize, float64(z) / cloudNoiseSize}
				value := func(f int, salt uint64) float64 {
					return cloudValue(p[0]*float64(f), p[1]*float64(f), p[2]*float64(f), f, seed+salt)
				}
				cell := cloudCell(p[0]*8, p[1]*8, p[2]*8, 8, seed+43)
				i := ((z*cloudNoiseSize+y)*cloudNoiseSize + x) * 4
				volume[i] = pack(0.58*value(4, 11) + 0.28*value(8, 17) + 0.14*value(16, 29))
				volume[i+1] = pack(cell)
				volume[i+2] = pack(0.7*value(2, 71) + 0.3*value(4, 97))
				volume[i+3] = 255
			}
		}
	}
	return packCloudNoise(volume)
}

func packCloudNoise(volume []byte) []byte {
	atlas := make([]byte, cloudNoiseWidth*cloudNoiseHeight*4)
	offset := 0
	for size := cloudNoiseSize; size >= 1; size /= 2 {
		tile, columns := size+2, min(8, size)
		for z := 0; z < size; z++ {
			for y := 0; y < tile; y++ {
				for x := 0; x < tile; x++ {
					xv, yv := (x+size-1)%size, (y+size-1)%size
					src := ((z*size+yv)*size + xv) * 4
					dst := ((offset+(z/columns)*tile+y)*cloudNoiseWidth + (z%columns)*tile + x) * 4
					copy(atlas[dst:dst+4], volume[src:src+4])
				}
			}
		}
		offset += ((size + columns - 1) / columns) * tile
		if size == 1 {
			break
		}
		n := size / 2
		next := make([]byte, n*n*n*4)
		for z := 0; z < n; z++ {
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					for c := 0; c < 4; c++ {
						sum := 0
						for dz := 0; dz < 2; dz++ {
							for dy := 0; dy < 2; dy++ {
								for dx := 0; dx < 2; dx++ {
									sum += int(volume[(((z*2+dz)*size+y*2+dy)*size+x*2+dx)*4+c])
								}
							}
						}
						next[((z*n+y)*n+x)*4+c] = byte((sum + 4) / 8)
					}
				}
			}
		}
		volume = next
	}
	return atlas
}
