package handlers

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVoteHashChain(t *testing.T) {
	at := time.Date(2026, 11, 20, 12, 0, 0, 123456000, time.UTC)
	g := genesisHash(1)
	h1 := voteHash(g, 1, 10, 100, "candidate", 5, "12", at)
	if h1 == g || len(h1) != 64 {
		t.Fatalf("hash inválido: %s", h1)
	}
	if voteHash(g, 1, 10, 100, "candidate", 5, "12", at) != h1 {
		t.Fatal("hash deve ser determinístico")
	}
	// qualquer alteração (ex.: trocar candidato) muda o hash
	if voteHash(g, 1, 10, 100, "candidate", 6, "12", at) == h1 {
		t.Fatal("alterar o candidato deveria alterar o hash")
	}
	if genesisHash(1) == genesisHash(2) {
		t.Fatal("gênese deve ser diferente por eleição")
	}
}

func TestCandidateValidation(t *testing.T) {
	cases := []struct {
		in candidateInput
		ok bool
	}{
		{candidateInput{Name: "Ana"}, true},
		{candidateInput{Name: "  "}, false},
		{candidateInput{Name: "Ana", PhotoURL: "http://x"}, false},
		{candidateInput{Name: "Ana", PhotoURL: "https://" + strings.Repeat("a", 500)}, false},
	}
	for i, c := range cases {
		if got := c.in.validate() == ""; got != c.ok {
			t.Errorf("caso %d: esperado ok=%v", i, c.ok)
		}
	}
}

func TestNextCandidateNumber(t *testing.T) {
	all := func(n int) (out []string) {
		for i := 1; i <= n; i++ {
			out = append(out, strconv.Itoa(i))
		}
		return
	}
	cases := []struct {
		used   []string
		digits int
		want   string
	}{
		{nil, 2, "01"},
		{[]string{"01", "02"}, 2, "03"},
		{[]string{"01", "03"}, 2, "04"},
		{[]string{"001"}, 3, "002"},
		{nil, 1, "1"},
		{[]string{"01", "99"}, 2, "02"},
		{all(9), 1, ""},
		{all(99), 2, ""},
		{all(99), 3, ""},
	}
	for i, c := range cases {
		got, ok := nextCandidateNumber(c.used, c.digits)
		if got != c.want || ok != (c.want != "") {
			t.Errorf("caso %d: esperado %q, veio %q", i, c.want, got)
		}
	}
}

func TestRenumber(t *testing.T) {
	cases := []struct {
		old    []string
		digits int
		want   string // vazio = não cabe
	}{
		{[]string{"01", "02", "05"}, 3, "001,002,005"},
		{[]string{"001", "012"}, 2, "01,12"},
		{[]string{"01", "03"}, 1, "1,3"},
		{[]string{"01", "12", "40"}, 1, "1,2,3"},
		{[]string{"02", "15", "01"}, 1, "2,3,1"},
		{strings.Fields("01 02 03 04 05 06 07 08 09 10"), 1, ""},
	}
	for i, c := range cases {
		got, ok := renumber(c.old, c.digits)
		if strings.Join(got, ",") != c.want || ok != (c.want != "") {
			t.Errorf("caso %d: esperado %q, veio %v", i, c.want, got)
		}
	}
}

func TestCopyTitle(t *testing.T) {
	if got := copyTitle("Eleição"); got != "Eleição (cópia)" {
		t.Fatal(got)
	}
	if got := copyTitle(strings.Repeat("é", 80)); len(got) > 160 || !strings.HasSuffix(got, " (cópia)") {
		t.Fatalf("título fora do limite: %d", len(got))
	}
}

func TestBrokenLink(t *testing.T) {
	link := func(id uint64, prev, hash string) chainLink {
		return chainLink{voteID: id, prev: prev, hash: hash, expected: hash}
	}
	// voto 1, voto 2, anulação do voto 1, voto 3
	ok := []chainLink{link(1, "g", "a"), link(2, "a", "b"), link(3, "c", "d"), link(1, "b", "c")}
	if id, broken := brokenLink("g", ok); broken {
		t.Fatalf("cadeia íntegra acusada no voto %d", id)
	}
	// anulação removida do meio da cadeia
	if id, broken := brokenLink("g", ok[:3]); !broken || id != 3 {
		t.Fatalf("remoção não detectada: %d %v", id, broken)
	}
	// dados alterados
	tampered := []chainLink{link(1, "g", "a"), {voteID: 2, prev: "a", hash: "b", expected: "x"}}
	if id, broken := brokenLink("g", tampered); !broken || id != 2 {
		t.Fatalf("alteração não detectada: %d %v", id, broken)
	}
	// dois elos apontando para o mesmo anterior
	if _, broken := brokenLink("g", []chainLink{link(1, "g", "a"), link(2, "g", "b")}); !broken {
		t.Fatal("bifurcação não detectada")
	}
}

func TestLoginLimiter(t *testing.T) {
	l := newLoginLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.allow("ip") {
			t.Fatal("deveria permitir")
		}
	}
	if l.allow("ip") {
		t.Fatal("deveria bloquear após 3 tentativas")
	}
	if !l.allow("outro-ip") {
		t.Fatal("limite é por IP")
	}
}

func TestVoterNormalize(t *testing.T) {
	v := voterInput{Name: "  Maria   da  Silva ", Document: " 001 "}
	if msg := v.normalize(); msg != "" || v.Name != "Maria da Silva" || v.Document != "001" {
		t.Fatalf("normalização falhou: %q %q %q", msg, v.Name, v.Document)
	}
}
