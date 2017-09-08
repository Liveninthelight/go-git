package packfile

const blksz = 16
const maxChainLength = 64

// deltaIndex is a modified version of JGit's DeltaIndex adapted to our current
// design.
type deltaIndex struct {
	table   []int
	entries []int
	mask    int
}

func (idx *deltaIndex) init(buf []byte) {
	scanner := newDeltaIndexScanner(buf, len(buf))
	idx.mask = scanner.mask
	idx.table = scanner.table
	idx.entries = make([]int, countEntries(scanner)+1)
	idx.copyEntries(scanner)
}

// findMatch returns the offset of src where the block starting at tgtOffset
// is and the length of the match. A length of 0 means there was no match. A
// length of -1 means the src length is lower than the blksz and whatever
// other positive length is the length of the match in bytes.
func (idx *deltaIndex) findMatch(src, tgt []byte, tgtOffset int) (srcOffset, l int) {
	if len(tgt) < tgtOffset+s {
		return 0, len(tgt) - tgtOffset
	}

	if len(src) < blksz {
		return 0, -1
	}

	if len(tgt) >= tgtOffset+s && len(src) >= blksz {
		h := hashBlock(tgt, tgtOffset)
		tIdx := h & idx.mask
		eIdx := idx.table[tIdx]
		if eIdx != 0 {
			srcOffset = idx.entries[eIdx]
		} else {
			return
		}

		l = matchLength(src, tgt, tgtOffset, srcOffset)
	}

	return
}

func matchLength(src, tgt []byte, otgt, osrc int) (l int) {
	lensrc := len(src)
	lentgt := len(tgt)
	for (osrc < lensrc && otgt < lentgt) && src[osrc] == tgt[otgt] {
		l++
		osrc++
		otgt++
	}
	return
}

func countEntries(scan *deltaIndexScanner) (cnt int) {
	// Figure out exactly how many entries we need. As we do the
	// enumeration truncate any delta chains longer than what we
	// are willing to scan during encode. This keeps the encode
	// logic linear in the size of the input rather than quadratic.
	for i := 0; i < len(scan.table); i++ {
		h := scan.table[i]
		if h == 0 {
			continue
		}

		size := 0
		for {
			size++
			if size == maxChainLength {
				scan.next[h] = 0
				break
			}
			h = scan.next[h]

			if h == 0 {
				break
			}
		}
		cnt += size
	}

	return
}

func (idx *deltaIndex) copyEntries(scanner *deltaIndexScanner) {
	// Rebuild the entries list from the scanner, positioning all
	// blocks in the same hash chain next to each other. We can
	// then later discard the next list, along with the scanner.
	//
	next := 1
	for i := 0; i < len(idx.table); i++ {
		h := idx.table[i]
		if h == 0 {
			continue
		}

		idx.table[i] = next
		for {
			idx.entries[next] = scanner.entries[h]
			next++
			h = scanner.next[h]

			if h == 0 {
				break
			}
		}
	}
}

type deltaIndexScanner struct {
	table   []int
	entries []int
	next    []int
	mask    int
	count   int
}

func newDeltaIndexScanner(buf []byte, size int) *deltaIndexScanner {
	size -= size % blksz
	worstCaseBlockCnt := size / blksz
	if worstCaseBlockCnt < 1 {
		return new(deltaIndexScanner)
	}

	tableSize := tableSize(worstCaseBlockCnt)
	scanner := &deltaIndexScanner{
		table:   make([]int, tableSize),
		mask:    tableSize - 1,
		entries: make([]int, worstCaseBlockCnt+1),
		next:    make([]int, worstCaseBlockCnt+1),
	}

	scanner.scan(buf, size)
	return scanner
}

// slightly modified version of JGit's DeltaIndexScanner. We store the offset on the entries
// instead of the entries and the key, so we avoid operations to retrieve the offset later, as
// we don't use the key.
// See: https://github.com/eclipse/jgit/blob/005e5feb4ecd08c4e4d141a38b9e7942accb3212/org.eclipse.jgit/src/org/eclipse/jgit/internal/storage/pack/DeltaIndexScanner.java
func (s *deltaIndexScanner) scan(buf []byte, end int) {
	lastHash := 0
	ptr := end - blksz

	for {
		key := hashBlock(buf, ptr)
		tIdx := key & s.mask
		head := s.table[tIdx]
		if head != 0 && lastHash == key {
			s.entries[head] = ptr
		} else {
			s.count++
			eIdx := s.count
			s.entries[eIdx] = ptr
			s.next[eIdx] = head
			s.table[tIdx] = eIdx
		}

		lastHash = key
		ptr -= blksz

		if 0 > ptr {
			break
		}
	}
}

func tableSize(worstCaseBlockCnt int) int {
	shift := 32 - leadingZeros(uint32(worstCaseBlockCnt))
	sz := 1 << uint(shift-1)
	if sz < worstCaseBlockCnt {
		sz <<= 1
	}
	return sz
}

// use https://golang.org/pkg/math/bits/#LeadingZeros32 in the future
func leadingZeros(x uint32) (n int) {
	if x >= 1<<16 {
		x >>= 16
		n = 16
	}
	if x >= 1<<8 {
		x >>= 8
		n += 8
	}
	n += int(len8tab[x])
	return 32 - n
}

var len8tab = [256]uint8{
	0x00, 0x01, 0x02, 0x02, 0x03, 0x03, 0x03, 0x03, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04,
	0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05, 0x05,
	0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
	0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06, 0x06,
	0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
	0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
	0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
	0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07, 0x07,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
	0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08, 0x08,
}

func hashBlock(raw []byte, ptr int) int {
	var hash uint32

	// The first 4 steps collapse out into a 4 byte big-endian decode,
	// with a larger right shift as we combined shift lefts together.
	//
	hash = ((uint32(raw[ptr]) & 0xff) << 24) |
		((uint32(raw[ptr+1]) & 0xff) << 16) |
		((uint32(raw[ptr+2]) & 0xff) << 8) |
		(uint32(raw[ptr+3]) & 0xff)
	hash ^= T[hash>>31]

	hash = ((hash << 8) | (uint32(raw[ptr+4]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+5]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+6]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+7]) & 0xff)) ^ T[hash>>23]

	hash = ((hash << 8) | (uint32(raw[ptr+8]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+9]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+10]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+11]) & 0xff)) ^ T[hash>>23]

	hash = ((hash << 8) | (uint32(raw[ptr+12]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+13]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+14]) & 0xff)) ^ T[hash>>23]
	hash = ((hash << 8) | (uint32(raw[ptr+15]) & 0xff)) ^ T[hash>>23]

	return int(hash)
}

var T = []uint32{0x00000000, 0xd4c6b32d, 0x7d4bd577,
	0xa98d665a, 0x2e5119c3, 0xfa97aaee, 0x531accb4, 0x87dc7f99,
	0x5ca23386, 0x886480ab, 0x21e9e6f1, 0xf52f55dc, 0x72f32a45,
	0xa6359968, 0x0fb8ff32, 0xdb7e4c1f, 0x6d82d421, 0xb944670c,
	0x10c90156, 0xc40fb27b, 0x43d3cde2, 0x97157ecf, 0x3e981895,
	0xea5eabb8, 0x3120e7a7, 0xe5e6548a, 0x4c6b32d0, 0x98ad81fd,
	0x1f71fe64, 0xcbb74d49, 0x623a2b13, 0xb6fc983e, 0x0fc31b6f,
	0xdb05a842, 0x7288ce18, 0xa64e7d35, 0x219202ac, 0xf554b181,
	0x5cd9d7db, 0x881f64f6, 0x536128e9, 0x87a79bc4, 0x2e2afd9e,
	0xfaec4eb3, 0x7d30312a, 0xa9f68207, 0x007be45d, 0xd4bd5770,
	0x6241cf4e, 0xb6877c63, 0x1f0a1a39, 0xcbcca914, 0x4c10d68d,
	0x98d665a0, 0x315b03fa, 0xe59db0d7, 0x3ee3fcc8, 0xea254fe5,
	0x43a829bf, 0x976e9a92, 0x10b2e50b, 0xc4745626, 0x6df9307c,
	0xb93f8351, 0x1f8636de, 0xcb4085f3, 0x62cde3a9, 0xb60b5084,
	0x31d72f1d, 0xe5119c30, 0x4c9cfa6a, 0x985a4947, 0x43240558,
	0x97e2b675, 0x3e6fd02f, 0xeaa96302, 0x6d751c9b, 0xb9b3afb6,
	0x103ec9ec, 0xc4f87ac1, 0x7204e2ff, 0xa6c251d2, 0x0f4f3788,
	0xdb8984a5, 0x5c55fb3c, 0x88934811, 0x211e2e4b, 0xf5d89d66,
	0x2ea6d179, 0xfa606254, 0x53ed040e, 0x872bb723, 0x00f7c8ba,
	0xd4317b97, 0x7dbc1dcd, 0xa97aaee0, 0x10452db1, 0xc4839e9c,
	0x6d0ef8c6, 0xb9c84beb, 0x3e143472, 0xead2875f, 0x435fe105,
	0x97995228, 0x4ce71e37, 0x9821ad1a, 0x31accb40, 0xe56a786d,
	0x62b607f4, 0xb670b4d9, 0x1ffdd283, 0xcb3b61ae, 0x7dc7f990,
	0xa9014abd, 0x008c2ce7, 0xd44a9fca, 0x5396e053, 0x8750537e,
	0x2edd3524, 0xfa1b8609, 0x2165ca16, 0xf5a3793b, 0x5c2e1f61,
	0x88e8ac4c, 0x0f34d3d5, 0xdbf260f8, 0x727f06a2, 0xa6b9b58f,
	0x3f0c6dbc, 0xebcade91, 0x4247b8cb, 0x96810be6, 0x115d747f,
	0xc59bc752, 0x6c16a108, 0xb8d01225, 0x63ae5e3a, 0xb768ed17,
	0x1ee58b4d, 0xca233860, 0x4dff47f9, 0x9939f4d4, 0x30b4928e,
	0xe47221a3, 0x528eb99d, 0x86480ab0, 0x2fc56cea, 0xfb03dfc7,
	0x7cdfa05e, 0xa8191373, 0x01947529, 0xd552c604, 0x0e2c8a1b,
	0xdaea3936, 0x73675f6c, 0xa7a1ec41, 0x207d93d8, 0xf4bb20f5,
	0x5d3646af, 0x89f0f582, 0x30cf76d3, 0xe409c5fe, 0x4d84a3a4,
	0x99421089, 0x1e9e6f10, 0xca58dc3d, 0x63d5ba67, 0xb713094a,
	0x6c6d4555, 0xb8abf678, 0x11269022, 0xc5e0230f, 0x423c5c96,
	0x96faefbb, 0x3f7789e1, 0xebb13acc, 0x5d4da2f2, 0x898b11df,
	0x20067785, 0xf4c0c4a8, 0x731cbb31, 0xa7da081c, 0x0e576e46,
	0xda91dd6b, 0x01ef9174, 0xd5292259, 0x7ca44403, 0xa862f72e,
	0x2fbe88b7, 0xfb783b9a, 0x52f55dc0, 0x8633eeed, 0x208a5b62,
	0xf44ce84f, 0x5dc18e15, 0x89073d38, 0x0edb42a1, 0xda1df18c,
	0x739097d6, 0xa75624fb, 0x7c2868e4, 0xa8eedbc9, 0x0163bd93,
	0xd5a50ebe, 0x52797127, 0x86bfc20a, 0x2f32a450, 0xfbf4177d,
	0x4d088f43, 0x99ce3c6e, 0x30435a34, 0xe485e919, 0x63599680,
	0xb79f25ad, 0x1e1243f7, 0xcad4f0da, 0x11aabcc5, 0xc56c0fe8,
	0x6ce169b2, 0xb827da9f, 0x3ffba506, 0xeb3d162b, 0x42b07071,
	0x9676c35c, 0x2f49400d, 0xfb8ff320, 0x5202957a, 0x86c42657,
	0x011859ce, 0xd5deeae3, 0x7c538cb9, 0xa8953f94, 0x73eb738b,
	0xa72dc0a6, 0x0ea0a6fc, 0xda6615d1, 0x5dba6a48, 0x897cd965,
	0x20f1bf3f, 0xf4370c12, 0x42cb942c, 0x960d2701, 0x3f80415b,
	0xeb46f276, 0x6c9a8def, 0xb85c3ec2, 0x11d15898, 0xc517ebb5,
	0x1e69a7aa, 0xcaaf1487, 0x632272dd, 0xb7e4c1f0, 0x3038be69,
	0xe4fe0d44, 0x4d736b1e, 0x99b5d833,
}
