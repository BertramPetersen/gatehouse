package update

import "testing"

func TestExtractBinaryFromTarGz(t *testing.T) {
	archive := makeTarGz(t, map[string][]byte{
		"nested/gatehouse": []byte("binary-bytes"),
	})

	binary, err := extractBinaryFromTarGz(archive, "gatehouse")
	if err != nil {
		t.Fatalf("extractBinaryFromTarGz error = %v", err)
	}
	if string(binary) != "binary-bytes" {
		t.Fatalf("binary = %q", string(binary))
	}

	_, err = extractBinaryFromTarGz(makeTarGz(t, map[string][]byte{"nested/other": []byte("x")}), "gatehouse")
	if err == nil {
		t.Fatal("extractBinaryFromTarGz should fail when binary is missing")
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	archive := makeZip(t, map[string][]byte{
		"nested/gatehouse.exe": []byte("binary-bytes"),
	})

	binary, err := extractBinaryFromZip(archive, "gatehouse.exe")
	if err != nil {
		t.Fatalf("extractBinaryFromZip error = %v", err)
	}
	if string(binary) != "binary-bytes" {
		t.Fatalf("binary = %q", string(binary))
	}

	_, err = extractBinaryFromZip(makeZip(t, map[string][]byte{"nested/other.exe": []byte("x")}), "gatehouse.exe")
	if err == nil {
		t.Fatal("extractBinaryFromZip should fail when binary is missing")
	}
}
