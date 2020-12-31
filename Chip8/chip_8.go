package Chip8

import (
	"fmt"
	"io/ioutil"
	"math/rand"

	"github.com/mellotonio/go-chip8/Chip8/Display"
)

// Uso de memória
// 0x000-0x1FF - Reservado para o interpretador do Chip-8 -> 0 ~ 512 (bits)
// 0x050-0x0A0 - "Used for the built in 4x5 pixel font set (0-F)"" -> 80 ~ 160 (bits)
// 0x200-0xFFF - Reservado para os programas e funcionalidades -> 512 ~ 4095 (bits)

type chip_8_VM struct {
	opcode          uint16        // Referência de instrução do processador
	memory          [4096]byte    // O Chip-8, originalmente, é capaz de acessar 4096 bytes de RAM (4KB)
	Vx              [16]byte      // Registradores de proposito geral, Vx aonde x é um hexadecimal z (0 até F)
	index           uint16        // Registrador de indice
	program_counter uint16        // Usado para guardar o endereço atual da instrução que está sendo executada (0x000 - 0 => 0xFFF - 4095)
	stack           [16]uint16    // Stack para "acumular" instruções
	stack_pointer   uint16        // Registro que guarda o ultimo endereço requisitado na pilha
	gfx             [64 * 32]byte // Pixels da tela
	key             [16]byte      // "16-key hexadecimal keypad for input"
	drawFlag        bool
}

// Inicialização do Chip8 com a fonte inicializada nos primeiros 80 bytes
func Start(pathToROM string) (*chip_8_VM, error) {

	chip8_INIT := chip_8_VM{
		memory:          [4096]byte{},
		Vx:              [16]byte{},
		program_counter: 0x200, // Começa no byte 512, já reservado para o inicio dos programas
		stack:           [16]uint16{},
		gfx:             [64 * 32]byte{},
		key:             [16]byte{},
	}

	chip8_INIT.loadFontSet()

	// Tenta iniciar a ROM
	if err := chip8_INIT.LoadROM(pathToROM); err != nil {
		return nil, err
	}

	return &chip8_INIT, nil

}

// Carrega a font nos primeiros 80 bytes de memoria
func (chip_8 *chip_8_VM) loadFontSet() {
	for i := 0; i < 80; i++ {
		chip_8.memory[i] = Display.FontSet[i]
	}
}

// Pega o caminho da ROM e carrega ela no Chip8
func (chip_8 *chip_8_VM) LoadROM(path string) error {
	rom, err := ioutil.ReadFile(path)

	if err != nil {
		return err
	}

	if len(rom) >= 3585 {
		panic("ERROR: ROM TOO LARGE - MAX SIZE: 3584") // Se a ROM ultrapassar o espaço dedicado para o interpretador ocorrerá um "panic"
	}

	for i := 0; i < len(rom); i++ {
		chip_8.memory[0x50+i] = rom[i] // Memoria começa 0x50 (80) + x, tirando espaço reservado para as fontes (80 bytes)
	}

	return nil
}

func (chip_8 *chip_8_VM) MachineCycle() {
	// Um opcode tem 2 bytes (16bit) de comprimento, por exemplo 0xA2F0 -> (0xA2 e 0xF0) -> e então transformar ele em um opcode válido
	// Primeiro temos de realizar uma operação de shift na instrução atual, ex: 10100010 - 8bit => 10100010 <<8 => 1010001000000000
	// Após isso temos de realizar uma operação OR para então termos os 16 bits necessarios para ser um opcode.

	// Operação OR vai pegar os "0" do lado direito e transformar no valor correspondente do byte
	chip_8.opcode = uint16(chip_8.memory[chip_8.program_counter])<<8 | uint16(chip_8.memory[chip_8.program_counter+1])
	chip_8.drawFlag = false

	chip_8.parseOpcode()
}

func (chip_8 *chip_8_VM) parseOpcode() {
	// Chip_8 Variables
	x := (chip_8.opcode & 0x0F00) >> 8 // 4 menores bits da instrução de maior nivel, como é um valor de 4 bits precisamos jogar ele pra ponta, tirando 8 zeros
	y := (chip_8.opcode & 0x00F0) >> 4 // 4 maiores bits da instrução de menor nivel, como é um valor de 4 bits precisamos jogar ele pra ponta, tirando 4 zeros
	nn := byte(chip_8.opcode & 0x00FF) // 8 menores bits da instrução
	nnn := chip_8.opcode & 0x0FFF      // 12 menores bits da instrução

	// BIT MASK - Most significant 4 bits -- "Precisa achar esta parte do hexa? F senão 0"
	// Ex.      1001111111111111
	//       &  1111000000000000 -> 0xF000
	// result: (1001)000000000000 ->  Esses 4 primeiros numeros servirão para dar match na operação que o processador deve realizar

	switch chip_8.opcode & 0xF000 {
	// 0NNN -> Execute machine language subroutine at address NNN
	case 0x0000:
		// Bit mask para os primeiros 8 bits - 11111111 == 255, ou seja, este switch abrange de 0 até 255
		// 4(0)4(0)4(1)4(1) - 0x00FF => 0000000011111111
		switch chip_8.opcode & 0x00FF {
		// Case 224
		case 0x00E0:
			// Comando que limpa a tela
			chip_8.gfx = [64 * 32]byte{}
			chip_8.program_counter += 2
		// Case 238
		case 0x00EE:
			// Retorna de uma subrotina
			// The interpreter sets the program counter to the address at the top of the stack, then subtracts 1 from the stack pointer.
			chip_8.program_counter = chip_8.stack[chip_8.stack_pointer] + 2
			chip_8.stack_pointer--
		}

	// ex: irá ser comparado os 4 primeiros recebidos do bitwise do opcode com os 4 primeiros desse case, no caso = (0001)...
	case 0x1000:
		// 1NNN -> Pula pro endereço nnn
		chip_8.program_counter = nnn
	case 0x2000:
		// 2NNN -> Executa subrotina começando no endereço NNN
		// The interpreter increments the stack pointer, then puts the current PC on the top of the stack. The PC is then set to nnn.
		chip_8.stack_pointer++
		chip_8.stack[chip_8.stack_pointer] = chip_8.program_counter
		chip_8.program_counter = nnn
	case 0x3000:
		// 3NNN -> Pula a proxima instrução se o valor do registrador Vx == NN
		// The interpreter increments the stack pointer, then puts the current PC on the top of the stack. The PC is then set to nnn.
		if chip_8.Vx[x] == nn {
			chip_8.program_counter += 4
		} else {
			chip_8.program_counter += 2
		}
	case 0x4000:
		// 4NNN -> Pula a proxima instrução se o valor do registrador Vx != NN
		// The interpreter compares register Vx to kk, and if they are not equal, increments the program counter by 2.

		if chip_8.Vx[x] != nn {
			chip_8.program_counter += 4
		} else {
			chip_8.program_counter += 2
		}
	case 0x5000:
		// 5XY0 -> Pula a proxima instrução se o valor do registrador Vx != Vy
		// The interpreter compares register Vx to register Vy, and if they are equal, increments the program counter by 2.
		if chip_8.Vx[x] == chip_8.Vx[y] {
			chip_8.program_counter += 4
		} else {
			chip_8.program_counter += 2
		}
	case 0x6000:
		// 6XNN -> Guarda o numero NN no registrador Vx
		// The interpreter puts the value kk into register Vx.
		chip_8.Vx[x] = nn
		chip_8.program_counter += 2
	case 0x7000:
		// 7XNN -> Adiciona o valor NN no registrador Vx
		// Adds the value kk to the value of register Vx, then stores the result in Vx.
		chip_8.Vx[x] += nn
		chip_8.program_counter += 2
	case 0x8000:
		// Bitmask para os primeiros 4 bits
		// Apos verificar se o opcode se encaixa no case 0x8000 (1000000000000000)
		// Temos que fazer outro switch pegando os primeiros 4 numeros (binario) do opcode
		// e comparar em qual instrução ele se encaixa
		switch chip_8.opcode & 0x000F {
		case 0x0000:
			// 8XY0 -> Guarda o valor do registrador Vy no registrador Vx
			chip_8.Vx[x] = chip_8.Vx[y]
			chip_8.program_counter += 2
		case 0x0001:
			// 8XY1 -> Transforma Vx em Vx ou Vy
			chip_8.Vx[x] |= chip_8.Vx[y]
			chip_8.program_counter += 2
		case 0x0002:
			// 8XY2 -> Transforma Vx em Vx e Vy
			chip_8.Vx[x] &= chip_8.Vx[y]
			chip_8.program_counter += 2
		case 0x0003:
			// 8XY3 -> Transforma Vx em Vx xor Vy
			chip_8.Vx[x] ^= chip_8.Vx[y]
			chip_8.program_counter += 2
		case 0x0004: // LEARN WHATS HAPPENING HERE ?
			// 8XY4 -> Set Vx = Vx + Vy, set VF = carry.
			// se o resultado for acima de 8 bits, a flag sera setada = 1, senão 0; Apenas os 8 "menores" bits sao mantidos no Vx
			if chip_8.Vx[y] > (0xFF - chip_8.Vx[x]) {
				chip_8.Vx[0xF] = 1
			} else {
				chip_8.Vx[0xF] = 0
			}
			chip_8.Vx[x] += chip_8.Vx[y]
			chip_8.program_counter += 2
		case 0x0005:
			// 8XY5 -> Set Vx = Vx - Vy, set VF = NOT borrow.
			// Se Vx > Vy, a flag sera setada = 1, senão a flag será 0, resultado guardado em Vx
			if chip_8.Vx[y] > chip_8.Vx[x] {
				chip_8.Vx[0xF] = 1
			} else {
				chip_8.Vx[0xF] = 0
			}
			chip_8.Vx[x] -= chip_8.Vx[y]
			chip_8.program_counter += 2
		case 0x0006:
			// 8XY6 -> Guarda o valor do registro Vy shifted 1 bit para direita no registro Vx
			// Seta a flag para o "least significant" bit no shift
			chip_8.Vx[x] = chip_8.Vx[y] >> 1 // divide by 2
			chip_8.Vx[0xF] = chip_8.Vx[y] & 0x01
			chip_8.program_counter += 2
		case 0x0007:
			// 8XY7 -> Set Vx = Vy - Vx, set VF = NOT borrow.
			// Vy > Vx, flag = 1, senão flag = 0, então Vx - Vy, guarda em Vx
			if chip_8.Vx[x] > chip_8.Vx[y] {
				chip_8.Vx[0xF] = 1
			} else {
				chip_8.Vx[0xF] = 0
			}
			chip_8.Vx[x] = chip_8.Vx[y] - chip_8.Vx[x]
			chip_8.program_counter += 2

		case 0x000E:
			// 8XYE -> Store the value of register VY shifted left one bit in register VX
			// Set register VF to the most significant bit prior to the shift
			chip_8.Vx[x] = chip_8.Vx[x] << 1     // multiply by 2
			chip_8.Vx[0xF] = chip_8.Vx[y] & 0x80 // most significant bit (bitmasking)
			chip_8.program_counter += 2

		default:
			fmt.Printf("unknown opcode: %x\n", chip_8.opcode&0x000F)
		}
	case 0x9000:
		// 9XY0 -> Pula a proxima instrução se o valor de Vx != valor de Vy
		if chip_8.Vx[x] != chip_8.Vx[y] {
			chip_8.program_counter += 4
		} else {
			chip_8.program_counter += 2
		}
	case 0xA000:
		// ANNN -> Guarda o endereço de memoria NNN no registro I(ndex)
		chip_8.index = nnn
		chip_8.program_counter += 2
	case 0xB000:
		// BNNN -> Pula para o endereço NNN	+ V0
		chip_8.program_counter = nnn + uint16(chip_8.Vx[0])
		chip_8.program_counter += 2
	case 0xC000:
		// CXNN -> Seta Vx como um numero aleatorio com a mascara de NN
		chip_8.Vx[x] = byte(rand.Float32()*255) & nn
		chip_8.program_counter += 2
	case 0xD000:
		// DXYN -> Desenha um sprite na posição Vx,Vy com N bytes, começando no endereço guardado no I(ndex)
		// Setar flag como 1 se tem pixels que serão "desligados", se não flag = 0

		// Borrowed from Chippy === to learn...
		x = uint16(chip_8.Vx[x])
		y = uint16(chip_8.Vx[y])

		var pix uint16
		height := chip_8.opcode & 0x000F // Pegamos o N do "DXYN"
		chip_8.Vx[0xF] = 0

		for yPoint := uint16(0); yPoint < height; yPoint++ {
			pix = uint16(chip_8.memory[chip_8.index+yPoint]) // Começamos no endereço que está no index
			for xPoint := uint16(0); xPoint < 8; xPoint++ {  // Each sprite is 8 units wide
				ind := (x + xPoint + ((y + yPoint) * 64))
				if ind > uint16(len(chip_8.GetGraphics())) {
					continue
				}
				if (pix & (0x80 >> xPoint)) != 0 { // ex: 1010101 & 1000000 -> 1010101 & 0100000 -> ....  verifica se cada pixel esta setado
					if chip_8.GetGraphics()[ind] == 1 { // Pixel Collision
						chip_8.Vx[0xF] = 1 // Set Collision
					}
					chip_8.gfx[ind] ^= 1 // wraps around the opposite side
				}
			}
		}

		chip_8.drawFlag = true
		chip_8.program_counter += 2
	case 0xE000:
		// Bitmask com 8 primeiros bits
		switch chip_8.opcode & 0x00FF {
		case 0x009E:
			// EX9E -> Pula a proxima instrução se a tecla correspondente ao valor que está no registro Vx é pressionada
		case 0x00A1:
			// EXA1 -> Pula a proxima instrução se a tecla correspondente ao valor que está no registro Vx não é pressionada
		}
	case 0xF000:
		// Bitmask com 8 primeiros bits
		switch chip_8.opcode & 0x00FF {
		case 0x0007:
			// FX07 -> Guarda o valor atual do delay timer no registrador Vx
		case 0x000A:
			// FX0A -> Aguarda uma tecla ser pressionada para guardar o resultado no registrador VX
		case 0x0015:
			// FX15 -> Seta o Delay timer para o valor do registro Vx
		case 0x0018:
			// FX18 -> Seta o valor do sound timer para o valor do registro Vx
		case 0x001E:
			// FX1E -> Adiciona o valor que esta no registrador Vx no registro I(ndex)
		case 0x0029:
			// FX29 -> Seta o Valor i(ndex) para o endereço de memoria do sprite correspondente ao digito hexadecimal guardado em Vx
		case 0x0033:
			// FX33 -> Store the binary-coded decimal equivalent of the value stored in register VX at addresses I, I+1, and I+2
		case 0x0055:
			// FX55 -> Store the values of registers V0 to VX inclusive in memory starting at address I
			// I is set to I + X + 1 after operation
		case 0x0065:
			// FX65 -> Fill registers V0 to VX inclusive with the values stored in memory starting at address I
			// I is set to I + X + 1 after operation
		default:
			fmt.Printf("unknown opcode: %x\n", chip_8.opcode&0x00FF)
		}
	default:
		fmt.Printf("unknown opcode: %x\n", chip_8.opcode&0x00FF)

	}
}

// GetGraphics TODO: doc
func (chip_8 *chip_8_VM) GetGraphics() [64 * 32]byte {
	return chip_8.gfx
}

func (chip_8 *chip_8_VM) debug() {
	fmt.Printf(`
	opcode: %x
	pc: %d
	sp: %d
	i: %d
	Registers:
	V0: %d
	V1: %d
	V2: %d
	V3: %d
	V4: %d
	V5: %d
	V6: %d
	V7: %d
	V8: %d
	V9: %d
	VA: %d
	VB: %d
	VC: %d
	VD: %d
	VE: %d
	VF: %d`,
		chip_8.opcode, chip_8.program_counter, chip_8.stack_pointer, chip_8.index,
		chip_8.Vx[0], chip_8.Vx[1], chip_8.Vx[2], chip_8.Vx[3],
		chip_8.Vx[4], chip_8.Vx[5], chip_8.Vx[6], chip_8.Vx[7],
		chip_8.Vx[8], chip_8.Vx[9], chip_8.Vx[10], chip_8.Vx[11],
		chip_8.Vx[12], chip_8.Vx[13], chip_8.Vx[14], chip_8.Vx[15],
	)
}
